/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
)

/*
Cluster identity, made checkable.

A cluster's identity is its transport CA root: every member's certificate chains
to it and to nothing else, so "same cluster" is a verifiable property, not a
naming convention. Two helpers build on that:

  - TransportCAFingerprint renders the identity as a stable sha256 fingerprint
    of the CA certificate — comparable at a glance across nodes, dashboards, and
    deployments (two clusters in one network have different fingerprints; a node
    restored from another cluster's backup has the same one, which is exactly
    the warning an operator needs).

  - PreflightClusterPeer proves a prospective member holds this cluster's
    identity BEFORE a membership change commits. Raft's mTLS would refuse a
    foreign or absent node anyway, but only after AddVoter has already recorded
    a phantom voter that counts toward quorum. The preflight moves that check
    up front: a full mutual-TLS handshake against the target's raft port — its
    certificate must chain to OUR CA (and carry the expected node id) — or the
    membership change is rejected with nothing committed.
*/

// TransportCAFingerprint is this node's cluster identity: the sha256
// fingerprint of the transport CA certificate in <dataDir>/pki/ca.pem.
// ok=false when the node has no cluster identity (single-node, or pre-genesis).
func TransportCAFingerprint(dataDir string) (string, bool) {
	pemBytes, err := os.ReadFile(filepath.Join(pkiDir(dataDir), caCertFile))
	if err != nil || len(pemBytes) == 0 {
		return "", false
	}
	fp, err := CAFingerprintFromPEM(pemBytes)
	return fp, err == nil
}

// CAFingerprintFromPEM fingerprints a PEM CA certificate: "sha256:<hex>" over
// the certificate's DER bytes (the standard certificate fingerprint).
func CAFingerprintFromPEM(pemBytes []byte) (string, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return "", xerrors.New("CA certificate PEM invalid")
	}
	sum := sha256.Sum256(block.Bytes)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// PreflightClusterPeer dials addr (a raft address, host:port) with this node's
// own mTLS identity and verifies the peer presents a certificate chaining to
// THIS cluster's CA; with nodeID non-empty the peer's certificate CN must also
// be that node id (a member can't be added under someone else's identity).
// A nil return means the target is a live, reachable member of this cluster.
func PreflightClusterPeer(dataDir, nodeID, addr string, timeout time.Duration) error {
	id, ok := LoadNodeIdentity(dataDir)
	if !ok {
		return xerrors.New("this node has no cluster identity to verify with (no pki/ material)")
	}
	cfg, err := id.MutualConfig()
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, cfg)
	if err != nil {
		return xerrors.Errorf("%s did not present this cluster's identity (certificate must chain to this cluster's CA): %w", addr, err)
	}
	defer conn.Close()
	if nodeID != "" {
		state := conn.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			return xerrors.Errorf("%s presented no certificate", addr)
		}
		if cn := state.PeerCertificates[0].Subject.CommonName; cn != nodeID {
			return xerrors.Errorf("%s identifies as node %q, not the requested %q", addr, cn, nodeID)
		}
	}
	return nil
}
