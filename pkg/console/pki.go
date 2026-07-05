/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"golang.org/x/xerrors"
)

// nodeCertTTL is how long an enrolled node certificate is valid; rotation is a
// re-enroll.
const nodeCertTTL = 5 * 365 * 24 * time.Hour

/*
The console owns consensusdb's built-in x509 certificate authority (pkg/iam/ca.go):
it creates the single CA on first use, hands out client certificates for users and
service accounts, and (in the node phase) mints join tokens and signs node certs.
The CA lives in the sealed system tenant (__system/PKI/ca), so writing it goes
through raft — the leader mints the one shared root and every node reads it locally.
*/

// loadCA reads and loads the built-in CA; ok=false when it has not been created.
func (t *ConsoleHandler) loadCA(ctx context.Context) (ca *iam.CA, ok bool, err error) {
	rec, err := t.svc.Get(ctx, &pb.KeyRequest{Key: iam.PKIKey(iam.CAMinor)})
	if err != nil {
		return nil, false, err
	}
	if rec == nil || len(rec.Value) == 0 {
		return nil, false, nil
	}
	car := &iam.CARecord{}
	if err := iam.Decode(rec.Value, car); err != nil {
		return nil, false, xerrors.Errorf("decode CA: %w", err)
	}
	loaded, err := car.Load()
	if err != nil {
		return nil, false, err
	}
	return loaded, true, nil
}

// ensureCA returns the built-in CA, creating it (create-if-absent) on first use.
// Creation is idempotent: a lost CAS race means another node won, so we re-read.
// Because the write routes through raft when replication is on, the seed/leader
// mints the single shared root and the private key is generated node-side, never
// travelling over the wire.
func (t *ConsoleHandler) ensureCA(ctx context.Context) (*iam.CA, error) {
	if ca, ok, err := t.loadCA(ctx); err != nil {
		return nil, err
	} else if ok {
		return ca, nil
	}
	fresh, err := iam.GenerateCA()
	if err != nil {
		return nil, err
	}
	raw, err := iam.Encode(fresh)
	if err != nil {
		return nil, err
	}
	status, err := t.svc.Put(ctx, &pb.RecordRequest{
		Key: iam.PKIKey(iam.CAMinor), Value: raw, CompareAndSet: true, Version: 0,
	})
	if err != nil {
		return nil, err
	}
	if status == nil || !status.Updated {
		// Lost the create race: another node minted the CA — read theirs.
		if ca, ok, err := t.loadCA(ctx); err == nil && ok {
			return ca, nil
		}
		return nil, xerrors.New("CA creation raced; re-read failed")
	}
	t.Log.Info("PkiCaCreated")
	return fresh.Load()
}

// ---------------------------------------------------------------------------
// Client certificates (users + service accounts)
// ---------------------------------------------------------------------------

type certOut struct {
	Identity  string `json:"identity"`
	Principal string `json:"principal"`
	Issued    bool   `json:"issued"` // issued by the built-in CA vs. externally registered
	CreatedAt int64  `json:"createdAt"`
}

// iamListCerts lists certificate identities, optionally filtered to one principal
// (?principal=user:alice). The cert index is the single source of truth for both
// users and service accounts.
func (t *ConsoleHandler) iamListCerts(w http.ResponseWriter, r *http.Request) {
	principal := r.URL.Query().Get("principal")
	out := []certOut{}
	err := t.scanIAM(func(minor string, value []byte) {
		ident, ok := strings.CutPrefix(minor, iam.CertPrefix)
		if !ok {
			return
		}
		idx := &iam.CertIndexRecord{}
		if iam.Decode(value, idx) == nil && (principal == "" || idx.Principal == principal) {
			out = append(out, certOut{ident, idx.Principal, idx.Issued, idx.CreatedAt})
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan certificates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"certs": out})
}

// iamRegisterCert maps an externally-issued certificate identity (a SAN URI or CN)
// to a principal — the register-only path: no key material is generated here, the
// owner already holds the certificate.
func (t *ConsoleHandler) iamRegisterCert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Principal string `json:"principal"`
		Identity  string `json:"identity"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	req.Identity = strings.TrimSpace(req.Identity)
	if req.Identity == "" || !t.principalExists(req.Principal) {
		writeErr(w, http.StatusBadRequest, "identity and an existing principal are required")
		return
	}
	if err := t.putCertIndex(req.Identity, req.Principal, false); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"identity": req.Identity, "principal": req.Principal})
}

// iamIssueCert issues a client certificate from the built-in CA for a principal:
// the console mints the keypair, the CA signs it (SAN URI = the principal's canonical
// cert URI), and the identity is registered. The private key is returned once, for
// the operator to download and hand to the owner.
func (t *ConsoleHandler) iamIssueCert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Principal string `json:"principal"`
		TTLDays   int    `json:"ttlDays"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !t.principalExists(req.Principal) {
		writeErr(w, http.StatusBadRequest, "an existing principal is required")
		return
	}
	_, name, _ := iam.ParsePrincipal(req.Principal)
	ttl := 365 * 24 * time.Hour
	if req.TTLDays > 0 {
		ttl = time.Duration(req.TTLDays) * 24 * time.Hour
	}
	ca, err := t.ensureCA(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "certificate authority: "+err.Error())
		return
	}
	keyPEM, pub, err := iam.GenerateLeafKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	uri := iam.CertURIForPrincipal(req.Principal)
	certPEM, err := ca.Sign(&iam.CertRequest{
		PublicKey: pub, CommonName: name, URIs: []string{uri}, Client: true, TTL: ttl,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sign certificate: "+err.Error())
		return
	}
	if err := t.putCertIndex(uri, req.Principal, true); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"identity": uri,
		"certPem":  string(certPEM),
		"keyPem":   string(keyPEM),
		"caPem":    string(ca.CertPEM),
		"note":     "the private key is shown once — download and store it now",
	})
}

// iamRevokeCert removes a certificate identity's mTLS mapping; the certificate no
// longer authenticates as any principal.
func (t *ConsoleHandler) iamRevokeCert(w http.ResponseWriter, identity string) {
	if identity == "" {
		writeErr(w, http.StatusBadRequest, "identity required")
		return
	}
	if _, err := t.svc.Remove(context.Background(), &pb.KeyRequest{Key: iam.Key(iam.CertPrefix + identity)}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": identity})
}

func (t *ConsoleHandler) putCertIndex(identity, principal string, issued bool) error {
	raw, err := iam.Encode(&iam.CertIndexRecord{Principal: principal, Issued: issued, CreatedAt: time.Now().Unix()})
	if err != nil {
		return err
	}
	_, err = t.svc.Put(context.Background(), &pb.RecordRequest{Key: iam.Key(iam.CertPrefix + identity), Value: raw})
	return err
}

// principalExists reports whether a user or service account record backs the
// principal.
func (t *ConsoleHandler) principalExists(principal string) bool {
	minor, ok := iam.PrincipalStorageMinor(principal)
	if !ok {
		return false
	}
	rec, err := t.svc.Get(context.Background(), &pb.KeyRequest{Key: iam.Key(minor)})
	return err == nil && rec != nil && len(rec.Value) > 0
}

// ---------------------------------------------------------------------------
// Cluster enrollment: join tokens + node certificate signing
// ---------------------------------------------------------------------------

// mintJoinToken creates a single-use, expiring join token authorizing one node to
// enroll; only its hash is stored. The token is returned once. Both the console
// ("Add node") and the CLI (`consensusdb cluster join-token`) mint the same record.
func (t *ConsoleHandler) mintJoinToken(ctx context.Context, ttl time.Duration, createdBy string) (token string, expiresAt int64, err error) {
	if _, err = t.ensureCA(ctx); err != nil { // a node can only enroll if the CA exists
		return "", 0, err
	}
	token, hash, err := iam.NewToken(iam.TokenPrefixJoin)
	if err != nil {
		return "", 0, err
	}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).Unix()
	}
	raw, err := iam.Encode(&iam.JoinRecord{ExpiresAt: expiresAt, CreatedBy: createdBy})
	if err != nil {
		return "", 0, err
	}
	if _, err = t.svc.Put(ctx, &pb.RecordRequest{Key: iam.PKIKey(iam.JoinIndexKey(hash)), Value: raw}); err != nil {
		return "", 0, err
	}
	return token, expiresAt, nil
}

// signNodeEnrollment verifies a join token and signs the node's CSR into a node
// certificate (server+client EKU, chaining to the built-in CA), with the node id
// and the given hosts as SANs. It is single-use: the join record is consumed on
// success. It does not touch raft membership — the caller adds the voter.
func (t *ConsoleHandler) signNodeEnrollment(ctx context.Context, token, nodeID string, hosts []string, csrPEM []byte) (certPEM, caPEM []byte, err error) {
	if token == "" || nodeID == "" {
		return nil, nil, xerrors.New("token and nodeId are required")
	}
	hash := iam.HashToken(token)
	rec, err := t.svc.Get(ctx, &pb.KeyRequest{Key: iam.PKIKey(iam.JoinIndexKey(hash))})
	if err != nil {
		return nil, nil, err
	}
	if rec == nil || len(rec.Value) == 0 {
		return nil, nil, xerrors.New("invalid join token")
	}
	jr := &iam.JoinRecord{}
	if err := iam.Decode(rec.Value, jr); err != nil {
		return nil, nil, err
	}
	if jr.ExpiresAt != 0 && time.Now().Unix() > jr.ExpiresAt {
		return nil, nil, xerrors.New("join token expired")
	}
	csr, err := iam.ParseCSR(csrPEM)
	if err != nil {
		return nil, nil, err
	}
	ca, err := t.ensureCA(ctx)
	if err != nil {
		return nil, nil, err
	}
	dns, ips := splitHosts(append([]string{nodeID}, hosts...))
	certPEM, err = ca.Sign(&iam.CertRequest{
		PublicKey: csr.PublicKey, CommonName: nodeID,
		DNSNames: dns, IPs: ips, Server: true, Client: true, TTL: nodeCertTTL,
	})
	if err != nil {
		return nil, nil, err
	}
	return certPEM, ca.CertPEM, nil
}

// consumeJoinToken deletes a join token's record (single-use). Called only after
// the node is successfully added, so a transient failure does not waste the token.
func (t *ConsoleHandler) consumeJoinToken(ctx context.Context, token string) {
	_, _ = t.svc.Remove(ctx, &pb.KeyRequest{Key: iam.PKIKey(iam.JoinIndexKey(iam.HashToken(token)))})
}

// splitHosts sorts host strings into IP-address and DNS-name SANs, de-duplicating.
func splitHosts(hosts []string) (dns []string, ips []net.IP) {
	seen := map[string]bool{}
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h == "" || seen[h] {
			continue
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		} else {
			dns = append(dns, h)
		}
	}
	return dns, ips
}
