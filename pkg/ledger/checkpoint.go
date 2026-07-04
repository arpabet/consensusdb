/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"encoding/binary"
	"sort"

	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

/*
A Checkpoint snapshots the hash-chain head at a committed height. A quorum of
nodes signs it (each with its BLS key, binding its node id), and the aggregate of
those signatures is a QuorumCertificate: cryptographic, offline-checkable evidence
that a majority of the certified cluster agreed on exactly this history up to this
height. This is the consensus made visible — verify it with only the CA public
key, the node certs, and the checkpoint; no running service, no vendor.
*/

// Checkpoint is the signed statement of the chain head at a height.
type Checkpoint struct {
	Height uint64 `value:"height"` // raft index of the head entry
	Term   uint64 `value:"term"`   // raft term at that height
	Digest []byte `value:"digest"` // 32-byte chain digest at Height
	Unix   int64  `value:"unix"`   // wall clock when proposed (advisory)
}

// Bytes is the canonical checkpoint encoding that goes into every signature.
func (c *Checkpoint) Bytes() []byte {
	b := make([]byte, 0, 8+8+len(c.Digest)+8)
	var u [8]byte
	binary.BigEndian.PutUint64(u[:], c.Height)
	b = append(b, u[:]...)
	binary.BigEndian.PutUint64(u[:], c.Term)
	b = append(b, u[:]...)
	b = append(b, c.Digest...)
	binary.BigEndian.PutUint64(u[:], uint64(c.Unix))
	return append(b, u[:]...)
}

// signMessage is what node nodeID signs to attest a checkpoint. Binding the node
// id makes each signer's message distinct (rogue-key-safe aggregation) while all
// signers still attest the same checkpoint bytes.
func signMessage(c *Checkpoint, nodeID string) []byte {
	m := append([]byte(domainCkpt), 0)
	m = append(m, c.Bytes()...)
	m = append(m, 0)
	return append(m, nodeID...)
}

// SignCheckpoint produces one node's attestation of a checkpoint.
func SignCheckpoint(key *NodePrivateKey, nodeID string, c *Checkpoint) []byte {
	return key.Sign(signMessage(c, nodeID))
}

// QuorumCertificate is a checkpoint plus the aggregate of its signers'
// attestations. Its on-the-wire size is the checkpoint, the small signer-id list,
// and one 48-byte aggregate signature — independent of cluster size.
type QuorumCertificate struct {
	Checkpoint Checkpoint `value:"checkpoint"`
	Signers    []string   `value:"signers"` // node ids whose signatures are aggregated
	AggSig     []byte     `value:"aggSig"`  // 48-byte BLS aggregate over the signers
}

// BuildQuorumCertificate aggregates per-node signatures into a certificate. sigs
// is keyed by node id (as returned by SignCheckpoint). Signers are sorted so the
// certificate bytes are deterministic.
func BuildQuorumCertificate(c *Checkpoint, sigs map[string][]byte) (*QuorumCertificate, error) {
	if len(sigs) == 0 {
		return nil, xerrors.New("ledger: no signatures for quorum certificate")
	}
	ids := make([]string, 0, len(sigs))
	for id := range sigs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ordered := make([][]byte, len(ids))
	for i, id := range ids {
		ordered[i] = sigs[id]
	}
	agg, err := Aggregate(ordered)
	if err != nil {
		return nil, err
	}
	return &QuorumCertificate{Checkpoint: *c, Signers: ids, AggSig: agg}, nil
}

// VerifyQuorum verifies a certificate against the CA: every signer must have a
// valid CA-issued cert, the aggregate signature must verify over exactly those
// signers' attestations, and the signer count must meet the threshold (e.g. a
// majority of the cluster). certs maps node id → its NodeCert. now is the current
// unix time for cert-expiry checks (0 to skip).
func VerifyQuorum(ca *CAPublicKey, qc *QuorumCertificate, certs map[string]*NodeCert, threshold int, now int64) error {
	if qc == nil || len(qc.Signers) == 0 {
		return xerrors.New("ledger: empty quorum certificate")
	}
	if len(qc.Signers) < threshold {
		return xerrors.Errorf("ledger: quorum not met: %d signers < threshold %d", len(qc.Signers), threshold)
	}
	// Reject duplicate signers (a padded signer list must not inflate the count).
	seen := make(map[string]struct{}, len(qc.Signers))
	pubs := make([]*NodePublicKey, 0, len(qc.Signers))
	msgs := make([][]byte, 0, len(qc.Signers))
	for _, id := range qc.Signers {
		if _, dup := seen[id]; dup {
			return xerrors.Errorf("ledger: duplicate signer %q", id)
		}
		seen[id] = struct{}{}
		cert, ok := certs[id]
		if !ok {
			return xerrors.Errorf("ledger: no cert for signer %q", id)
		}
		pub, err := ca.Verify(cert, now)
		if err != nil {
			return xerrors.Errorf("ledger: signer %q: %w", id, err)
		}
		pubs = append(pubs, pub)
		msgs = append(msgs, signMessage(&qc.Checkpoint, id))
	}
	if !VerifyAggregate(pubs, msgs, qc.AggSig) {
		return xerrors.New("ledger: aggregate signature invalid")
	}
	return nil
}

// Encode / Decode a quorum certificate (value canonical encoding).
func EncodeQuorum(qc *QuorumCertificate) ([]byte, error) {
	v, err := value.Marshal(qc)
	if err != nil {
		return nil, err
	}
	return value.Pack(v)
}

func DecodeQuorum(raw []byte) (*QuorumCertificate, error) {
	v, err := value.Unpack(raw, true)
	if err != nil {
		return nil, err
	}
	qc := &QuorumCertificate{}
	if err := value.Unmarshal(v, qc); err != nil {
		return nil, err
	}
	return qc, nil
}
