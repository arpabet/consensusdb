/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/pb"
)

/*
Verification-material collection for the "Verify Ledger" console page.

The verify engine deliberately takes its inputs — CA public key, quorum
certificate, node certs — as data, so verification works offline against a trust
anchor obtained out-of-band (a cluster must not be the source of the key that
vouches for the cluster). But an admin AT the console is usually doing an
operational consistency check, and every input except the trust anchor is the
cluster's to produce: node certs travel with each node's signed digest, and the
quorum certificate is an aggregation this coordinator can drive. These endpoints
close that gap:

  GET  /api/ledger/digest     one node's attestation (co-signs a requested head)
  GET  /api/ledger/materials  collect digests from every member, aggregate the
                              quorum certificate, return it with the node certs
                              and the pinned CA public key
  GET/POST /api/ledger/ca-pub the pinned CA PUBLIC key (set once by an admin, or
                              automatically by the onboarding CA generator)

Aggregation needs every signer over the same canonical checkpoint bytes, so the
coordinator reads its own head, stamps one checkpoint, and asks each member to
co-sign exactly that (a node signs only when the head is also its own — see
LedgerService.Attest). Materials produced this way are labeled for what they
are: convenient for consistency checks, not an independent audit.
*/

// LedgerAttester is the slice of replication.LedgerService the console needs;
// an interface so tests can stub a signer without the full raft stack.
type LedgerAttester interface {
	Attest(want *ledger.Checkpoint) (ckpt *ledger.Checkpoint, nodeID string, sig, cert []byte, signed bool)
}

// digestOut is one node's attestation on the wire (console REST form).
type digestOut struct {
	NodeID    string `json:"nodeId,omitempty"`
	Height    uint64 `json:"height"`
	Term      uint64 `json:"term"`
	Digest    string `json:"digest"` // hex, like the dashboard's chain head
	Unix      int64  `json:"unix"`
	Signature string `json:"signature,omitempty"` // base64
	Cert      string `json:"cert,omitempty"`      // base64 NodeCert
	Signed    bool   `json:"signed"`
}

// ledgerDigest returns this node's attestation. With height/digest (and
// unix/term) query params it co-signs that exact checkpoint when it matches the
// node's own head; without them it reports the node's head standalone.
func (t *ConsoleHandler) ledgerDigest(w http.ResponseWriter, r *http.Request) {
	if t.Ledger == nil {
		writeErr(w, http.StatusConflict, "the ledger service runs in cluster mode only")
		return
	}
	ckpt, nodeID, sig, cert, signed := t.Ledger.Attest(checkpointFromQuery(r))
	writeJSON(w, http.StatusOK, digestOut{
		NodeID: nodeID, Height: ckpt.Height, Term: ckpt.Term,
		Digest: hex.EncodeToString(ckpt.Digest), Unix: ckpt.Unix,
		Signature: base64.StdEncoding.EncodeToString(sig),
		Cert:      base64.StdEncoding.EncodeToString(cert),
		Signed:    signed,
	})
}

func checkpointFromQuery(r *http.Request) *ledger.Checkpoint {
	q := r.URL.Query()
	if q.Get("height") == "" || q.Get("digest") == "" {
		return nil
	}
	height, err1 := strconv.ParseUint(q.Get("height"), 10, 64)
	digest, err2 := hex.DecodeString(q.Get("digest"))
	if err1 != nil || err2 != nil {
		return nil
	}
	term, _ := strconv.ParseUint(q.Get("term"), 10, 64)
	unix, _ := strconv.ParseInt(q.Get("unix"), 10, 64)
	return &ledger.Checkpoint{Height: height, Term: term, Digest: digest, Unix: unix}
}

/*
ledgerMaterials assembles everything the verify form needs from the live
cluster: it stamps one checkpoint of this node's head, collects each member's
co-signature of exactly that checkpoint (retrying once when heads move between
calls), aggregates them into a quorum certificate, and returns it with the node
certs and the pinned CA public key. Unsigned or diverged members are reported in
warnings rather than failing the whole request — partial materials still tell
the admin what the cluster can currently prove.
*/
func (t *ConsoleHandler) ledgerMaterials(w http.ResponseWriter, r *http.Request) {
	if t.Ledger == nil {
		writeErr(w, http.StatusConflict, "the ledger service runs in cluster mode only")
		return
	}
	var (
		out struct {
			Height     uint64   `json:"height"`
			Digest     string   `json:"digest"`
			Unix       int64    `json:"unix"`
			Signers    []string `json:"signers"`
			Members    int      `json:"members"`
			QuorumCert string   `json:"quorumCert,omitempty"`
			NodeCerts  []string `json:"nodeCerts,omitempty"`
			CaPub      string   `json:"caPub,omitempty"`
			Warnings   []string `json:"warnings,omitempty"`
			Note       string   `json:"note"`
		}
		sigs  map[string][]byte
		certs [][]byte
		ckpt  *ledger.Checkpoint
	)

	// Two passes: when a member's head moved past ours mid-collection, restamp
	// from our (by then replicated) head and try once more.
	for attempt := 0; attempt < 2; attempt++ {
		own, _, _, _, _ := t.Ledger.Attest(nil)
		ckpt = own
		sigs, certs = map[string][]byte{}, nil
		out.Warnings, out.Signers = nil, nil

		members := t.ledgerMembers()
		out.Members = len(members)
		diverged := false
		for _, m := range members {
			d, err := t.collectDigest(r, m, ckpt)
			if err != nil {
				out.Warnings = append(out.Warnings, m.label+": "+err.Error())
				continue
			}
			if !d.Signed {
				if d.Height != ckpt.Height || d.Digest != hex.EncodeToString(ckpt.Digest) {
					diverged = true
					out.Warnings = append(out.Warnings, m.label+": head mismatch (height "+strconv.FormatUint(d.Height, 10)+")")
				} else {
					out.Warnings = append(out.Warnings, m.label+": no ledger signer configured (ledger.node-key / ledger.node-cert)")
				}
				continue
			}
			sig, err1 := base64.StdEncoding.DecodeString(d.Signature)
			cert, err2 := base64.StdEncoding.DecodeString(d.Cert)
			if err1 != nil || err2 != nil || d.NodeID == "" {
				out.Warnings = append(out.Warnings, m.label+": malformed attestation")
				continue
			}
			sigs[d.NodeID] = sig
			certs = append(certs, cert)
			out.Signers = append(out.Signers, d.NodeID)
		}
		if !diverged {
			break
		}
	}

	out.Height, out.Unix, out.Digest = ckpt.Height, ckpt.Unix, hex.EncodeToString(ckpt.Digest)
	if len(sigs) > 0 {
		qc, err := ledger.BuildQuorumCertificate(ckpt, sigs)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "aggregate quorum certificate: "+err.Error())
			return
		}
		raw, err := ledger.EncodeQuorum(qc)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "encode quorum certificate: "+err.Error())
			return
		}
		out.QuorumCert = base64.StdEncoding.EncodeToString(raw)
		for _, c := range certs {
			out.NodeCerts = append(out.NodeCerts, base64.StdEncoding.EncodeToString(c))
		}
	} else {
		out.Warnings = append(out.Warnings, "no signed attestations: the quorum certificate could not be built")
	}
	if pub := t.pinnedLedgerCAPub(); len(pub) > 0 {
		out.CaPub = base64.StdEncoding.EncodeToString(pub)
	} else {
		out.Warnings = append(out.Warnings, "no ledger CA public key pinned: paste it, or POST /api/ledger/ca-pub once")
	}
	out.Note = "cluster-supplied materials support a consistency check; an independent audit must take the CA public key (and ideally the quorum certificate) from records kept outside the cluster"
	writeJSON(w, http.StatusOK, out)
}

// ledgerMember is one collection target: the local node, or a peer's console.
type ledgerMember struct {
	label string
	self  bool
	host  string // peer console host (raft address host); http port is shared
}

// ledgerMembers lists collection targets from the raft configuration; with
// replication off (or before a configuration exists) it is just this node.
func (t *ConsoleHandler) ledgerMembers() []ledgerMember {
	rf, ok := t.raftHandle()
	if !ok {
		return []ledgerMember{{label: "local", self: true}}
	}
	cfg := rf.GetConfiguration()
	if err := cfg.Error(); err != nil || len(cfg.Configuration().Servers) == 0 {
		return []ledgerMember{{label: "local", self: true}}
	}
	selfID := t.selfNodeID(rf)
	var members []ledgerMember
	for _, s := range cfg.Configuration().Servers {
		if string(s.ID) == selfID {
			members = append(members, ledgerMember{label: string(s.ID), self: true})
			continue
		}
		members = append(members, ledgerMember{label: string(s.ID), host: hostOnly(string(s.Address))})
	}
	return members
}

// collectDigest fetches one member's attestation of ckpt: in-process for this
// node, over the member's console REST (forwarding the caller's Authorization,
// like proxyToLeader) for peers.
func (t *ConsoleHandler) collectDigest(r *http.Request, m ledgerMember, ckpt *ledger.Checkpoint) (*digestOut, error) {
	if m.self {
		got, nodeID, sig, cert, signed := t.Ledger.Attest(ckpt)
		return &digestOut{
			NodeID: nodeID, Height: got.Height, Digest: hex.EncodeToString(got.Digest),
			Signature: base64.StdEncoding.EncodeToString(sig),
			Cert:      base64.StdEncoding.EncodeToString(cert), Signed: signed,
		}, nil
	}
	url := "http://" + net.JoinHostPort(m.host, t.httpPort()) +
		"/api/ledger/digest?height=" + strconv.FormatUint(ckpt.Height, 10) +
		"&digest=" + hex.EncodeToString(ckpt.Digest) +
		"&unix=" + strconv.FormatInt(ckpt.Unix, 10) +
		"&term=" + strconv.FormatUint(ckpt.Term, 10)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if a := r.Header.Get("Authorization"); a != "" {
		req.Header.Set("Authorization", a)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	d := &digestOut{}
	if err := json.NewDecoder(resp.Body).Decode(d); err != nil {
		return nil, err
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Pinned ledger CA public key
// ---------------------------------------------------------------------------

// pinnedLedgerCAPub reads the stored CA public key; nil when none is pinned.
func (t *ConsoleHandler) pinnedLedgerCAPub() []byte {
	rec, err := t.svc.Get(context.Background(), &pb.KeyRequest{Key: iam.PKIKey(iam.LedgerCAPubMinor)})
	if err != nil || rec == nil {
		return nil
	}
	return rec.Value
}

// pinLedgerCAPub stores the CA PUBLIC key (raw Ed25519 bytes) as a replicated
// record so the verify form can be prefilled. Never the private key.
func (t *ConsoleHandler) pinLedgerCAPub(pub []byte) error {
	_, err := t.svc.Put(context.Background(), &pb.RecordRequest{
		Key: iam.PKIKey(iam.LedgerCAPubMinor), Value: pub,
	})
	return err
}

func (t *ConsoleHandler) ledgerCAPubGet(w http.ResponseWriter) {
	pub := t.pinnedLedgerCAPub()
	writeJSON(w, http.StatusOK, map[string]any{
		"caPub":  base64.StdEncoding.EncodeToString(pub),
		"pinned": len(pub) > 0,
	})
}

func (t *ConsoleHandler) ledgerCAPubSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CaPub string `json:"caPub"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	pub, err := base64.StdEncoding.DecodeString(req.CaPub)
	if err == nil {
		_, err = ledger.ParseCAPublicKey(pub)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "caPub must be the base64 Ed25519 ledger-CA public key")
		return
	}
	if err := t.pinLedgerCAPub(pub); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pinned": true})
}
