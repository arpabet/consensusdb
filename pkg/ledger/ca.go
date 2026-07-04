/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"crypto/ed25519"
	"encoding/binary"

	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

/*
The ledger CA is a small Ed25519 authority that vouches for node signing keys: a
NodeCert binds a node id to its BLS public key, signed by the CA. Verifiers trust
a node's checkpoint signatures only via a CA-signed cert, so the set of legitimate
signers is exactly the set the CA has certified. Issuing a cert requires a
proof-of-possession (the node signs the cert body with its BLS key), which — with
CA gatekeeping — defeats rogue-key attacks on the aggregate.

Ed25519 keeps the CA itself tiny (32-byte keys, 64-byte signatures) and offline:
the CA private key never needs to touch a running node.
*/

const (
	domainCert = "cdb/ledger/nodecert/v1"
	domainPoP  = "cdb/ledger/pop/v1"
	domainCkpt = "cdb/ledger/checkpoint/v1"
)

// CA is the ledger certificate authority (Ed25519).
type CA struct {
	priv ed25519.PrivateKey
}

// CAPublicKey verifies node certs; it is what an offline auditor needs (plus the
// certs and checkpoint) to verify a quorum — nothing else, no running service.
type CAPublicKey struct {
	pub ed25519.PublicKey
}

// GenerateCA creates a new ledger CA.
func GenerateCA() (*CA, error) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	return &CA{priv: priv}, nil
}

// Public returns the CA public key.
func (c *CA) Public() *CAPublicKey { return &CAPublicKey{pub: c.priv.Public().(ed25519.PublicKey)} }

// MarshalBinary / ParseCA persist the CA private seed.
func (c *CA) MarshalBinary() ([]byte, error) { return c.priv.Seed(), nil }

func ParseCA(seed []byte) (*CA, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, xerrors.Errorf("ledger: CA seed must be %d bytes", ed25519.SeedSize)
	}
	return &CA{priv: ed25519.NewKeyFromSeed(seed)}, nil
}

func (p *CAPublicKey) MarshalBinary() ([]byte, error) { return append([]byte(nil), p.pub...), nil }

func ParseCAPublicKey(data []byte) (*CAPublicKey, error) {
	if len(data) != ed25519.PublicKeySize {
		return nil, xerrors.Errorf("ledger: CA public key must be %d bytes", ed25519.PublicKeySize)
	}
	return &CAPublicKey{pub: append(ed25519.PublicKey(nil), data...)}, nil
}

// NodeCert binds a node id to its BLS public key, certified by the CA.
type NodeCert struct {
	NodeID    string `value:"nodeId"`
	PublicKey []byte `value:"publicKey"` // compressed BLS G2 public key
	NotAfter  int64  `value:"notAfter"`  // unix seconds; 0 = no expiry
	CASig     []byte `value:"caSig"`     // Ed25519 over certBody
}

// certBody is the exact bytes the CA signs (and the node PoP-signs).
func certBody(nodeID string, pub []byte, notAfter int64) []byte {
	b := append([]byte(domainCert), 0)
	b = append(b, nodeID...)
	b = append(b, 0)
	b = append(b, pub...)
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], uint64(notAfter))
	return append(b, t[:]...)
}

// popMessage is what a node signs with its BLS key to prove possession at issue.
func popMessage(nodeID string, pub []byte) []byte {
	b := append([]byte(domainPoP), 0)
	b = append(b, nodeID...)
	b = append(b, 0)
	return append(b, pub...)
}

// ProofOfPossession is the node's BLS signature over its own (id, pubkey), proving
// it controls the private key. The CA requires it before issuing a cert.
func ProofOfPossession(key *NodePrivateKey, nodeID string) ([]byte, error) {
	pub, err := key.Public().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return key.Sign(popMessage(nodeID, pub)), nil
}

// Issue certifies a node's BLS public key after verifying its proof-of-possession.
func (c *CA) Issue(nodeID string, nodePub *NodePublicKey, pop []byte, notAfter int64) (*NodeCert, error) {
	pub, err := nodePub.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if !nodePub.Verify(popMessage(nodeID, pub), pop) {
		return nil, xerrors.New("ledger: proof-of-possession failed")
	}
	sig := ed25519.Sign(c.priv, certBody(nodeID, pub, notAfter))
	return &NodeCert{NodeID: nodeID, PublicKey: pub, NotAfter: notAfter, CASig: sig}, nil
}

// Verify checks a node cert against the CA and (optionally) an expiry time; it
// returns the certified node public key.
func (p *CAPublicKey) Verify(cert *NodeCert, now int64) (*NodePublicKey, error) {
	if cert == nil {
		return nil, xerrors.New("ledger: nil cert")
	}
	if cert.NotAfter != 0 && now != 0 && now > cert.NotAfter {
		return nil, xerrors.New("ledger: node cert expired")
	}
	if !ed25519.Verify(p.pub, certBody(cert.NodeID, cert.PublicKey, cert.NotAfter), cert.CASig) {
		return nil, xerrors.New("ledger: node cert not signed by this CA")
	}
	return ParseNodePublicKey(cert.PublicKey)
}

// EncodeCert / DecodeCert (value canonical encoding, wire + on-disk).
func EncodeCert(cert *NodeCert) ([]byte, error) {
	v, err := value.Marshal(cert)
	if err != nil {
		return nil, err
	}
	return value.Pack(v)
}

func DecodeCert(raw []byte) (*NodeCert, error) {
	v, err := value.Unpack(raw, true)
	if err != nil {
		return nil, err
	}
	cert := &NodeCert{}
	if err := value.Unmarshal(v, cert); err != nil {
		return nil, err
	}
	return cert, nil
}
