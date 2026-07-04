/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"crypto/rand"

	"github.com/cloudflare/circl/sign/bls"
	"golang.org/x/xerrors"
)

/*
Node signing keys are BLS12-381 with the public key in G2, so signatures land in
G1 — 48 bytes each, and any number of node signatures over a checkpoint aggregate
into a single 48-byte signature. That is the smallest quorum-certificate footprint
without a distributed key generation ceremony.

Rogue-key safety: aggregate verification binds each signer's node id into the
message it signs (distinct-message aggregation), and node public keys are only
trusted once certified by the CA (which requires a proof-of-possession at issue —
see ca.go), so an attacker cannot register a crafted aggregate key.
*/

// keyGroup fixes the public-key group; G2 pubkeys ⇒ G1 (48-byte) signatures.
type keyGroup = bls.G2

// SignatureSize is the compressed G1 signature length in bytes.
const SignatureSize = 48

// NodePrivateKey signs checkpoints on behalf of one node.
type NodePrivateKey struct {
	priv *bls.PrivateKey[keyGroup]
}

// NodePublicKey verifies a node's checkpoint signatures.
type NodePublicKey struct {
	pub *bls.PublicKey[keyGroup]
}

// GenerateNodeKey creates a fresh BLS node key pair.
func GenerateNodeKey() (*NodePrivateKey, error) {
	var ikm [32]byte
	if _, err := rand.Read(ikm[:]); err != nil {
		return nil, err
	}
	priv, err := bls.KeyGen[keyGroup](ikm[:], nil, nil)
	if err != nil {
		return nil, err
	}
	return &NodePrivateKey{priv: priv}, nil
}

// Public returns the matching public key.
func (k *NodePrivateKey) Public() *NodePublicKey { return &NodePublicKey{pub: k.priv.PublicKey()} }

// MarshalBinary returns the private scalar bytes.
func (k *NodePrivateKey) MarshalBinary() ([]byte, error) { return k.priv.MarshalBinary() }

// ParseNodePrivateKey loads a private key from MarshalBinary bytes.
func ParseNodePrivateKey(data []byte) (*NodePrivateKey, error) {
	priv := new(bls.PrivateKey[keyGroup])
	if err := priv.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return &NodePrivateKey{priv: priv}, nil
}

// MarshalBinary returns the compressed public key (96 bytes).
func (k *NodePublicKey) MarshalBinary() ([]byte, error) { return k.pub.MarshalBinary() }

// ParseNodePublicKey loads a public key from its compressed bytes and validates
// it (rejects the identity / subgroup-invalid points).
func ParseNodePublicKey(data []byte) (*NodePublicKey, error) {
	pub := new(bls.PublicKey[keyGroup])
	if err := pub.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	if !pub.Validate() {
		return nil, xerrors.New("ledger: invalid node public key")
	}
	return &NodePublicKey{pub: pub}, nil
}

// Sign produces this node's signature over msg (48-byte G1 point).
func (k *NodePrivateKey) Sign(msg []byte) []byte { return bls.Sign(k.priv, msg) }

// Verify checks a single node signature over msg.
func (k *NodePublicKey) Verify(msg, sig []byte) bool { return bls.Verify(k.pub, msg, sig) }

// Aggregate combines individual signatures into one. The order of sigs does not
// matter, but VerifyAggregate must be given the pubkeys and messages in the same
// order the signatures were produced/collected.
func Aggregate(sigs [][]byte) ([]byte, error) {
	if len(sigs) == 0 {
		return nil, xerrors.New("ledger: no signatures to aggregate")
	}
	var zero keyGroup
	return bls.Aggregate(zero, sigs)
}

// VerifyAggregate verifies that aggSig is the aggregate of each pub[i]'s
// signature over msgs[i]. Messages must be distinct per signer (we bind the node
// id into each), which is what makes same-checkpoint aggregation rogue-key-safe.
func VerifyAggregate(pubs []*NodePublicKey, msgs [][]byte, aggSig []byte) bool {
	if len(pubs) == 0 || len(pubs) != len(msgs) {
		return false
	}
	inner := make([]*bls.PublicKey[keyGroup], len(pubs))
	for i, p := range pubs {
		inner[i] = p.pub
	}
	return bls.VerifyAggregate(inner, msgs, aggSig)
}
