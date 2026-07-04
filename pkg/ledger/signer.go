/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"os"

	"golang.org/x/xerrors"
)

/*
NodeSigner is a node's on-disk ledger identity: its BLS private key and the
CA-issued cert binding that key to the node id. A node loads it at startup to
co-sign checkpoints. Both files are produced by the CLI:

	consensusdb ledger keygen        → writes the BLS key
	consensusdb ledger issue …       → the CA writes the cert (offline, with the
	                                   node's public key + proof-of-possession)
*/
type NodeSigner struct {
	key  *NodePrivateKey
	cert *NodeCert
}

// NodeID is the identity this signer signs as (from its cert).
func (s *NodeSigner) NodeID() string { return s.cert.NodeID }

// Cert returns the CA-issued node certificate (published so verifiers can check
// this node's signatures).
func (s *NodeSigner) Cert() *NodeCert { return s.cert }

// Sign attests a checkpoint as this node.
func (s *NodeSigner) Sign(c *Checkpoint) []byte { return SignCheckpoint(s.key, s.cert.NodeID, c) }

// LoadNodeSigner loads the BLS key and node cert from disk. It fails if the cert
// does not certify the loaded key (a mismatched key/cert pair would sign
// unverifiably).
func LoadNodeSigner(keyPath, certPath string) (*NodeSigner, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, xerrors.Errorf("ledger: read node key: %w", err)
	}
	key, err := ParseNodePrivateKey(keyBytes)
	if err != nil {
		return nil, xerrors.Errorf("ledger: parse node key: %w", err)
	}
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, xerrors.Errorf("ledger: read node cert: %w", err)
	}
	cert, err := DecodeCert(certBytes)
	if err != nil {
		return nil, xerrors.Errorf("ledger: parse node cert: %w", err)
	}
	// The cert must bind exactly this key.
	pub, err := key.Public().MarshalBinary()
	if err != nil {
		return nil, err
	}
	if string(pub) != string(cert.PublicKey) {
		return nil, xerrors.New("ledger: node cert does not certify this key")
	}
	return &NodeSigner{key: key, cert: cert}, nil
}
