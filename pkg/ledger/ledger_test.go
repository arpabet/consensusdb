/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"bytes"
	"testing"
)

// The chain is deterministic (same entries ⇒ same head) and order-sensitive
// (any change to any entry changes the head — tamper-evident).
func TestHashChainDeterministic(t *testing.T) {
	build := func(cmds [][]byte) [32]byte {
		c := NewHashChain(0, GenesisDigest)
		for i, cmd := range cmds {
			c.Advance(uint64(i+1), 1, cmd)
		}
		_, d := c.Head()
		return d
	}
	a := build([][]byte{[]byte("put x"), []byte("put y"), []byte("del x")})
	b := build([][]byte{[]byte("put x"), []byte("put y"), []byte("del x")})
	if a != b {
		t.Fatal("chain not deterministic")
	}
	tampered := build([][]byte{[]byte("put x"), []byte("put Y"), []byte("del x")})
	if a == tampered {
		t.Fatal("a changed entry must change the head")
	}
	reordered := build([][]byte{[]byte("put y"), []byte("put x"), []byte("del x")})
	if a == reordered {
		t.Fatal("reordering must change the head")
	}

	// Advance ignores replays and out-of-order indices (idempotent apply).
	c := NewHashChain(0, GenesisDigest)
	c.Advance(1, 1, []byte("a"))
	_, d1 := c.Head()
	c.Advance(1, 1, []byte("a")) // replay
	c.Advance(0, 1, []byte("z")) // stale
	_, d2 := c.Head()
	if d1 != d2 {
		t.Fatal("replayed/stale entries must not advance the head")
	}
}

func TestBLSSignAggregate(t *testing.T) {
	k1, _ := GenerateNodeKey()
	k2, _ := GenerateNodeKey()
	m1, m2 := []byte("attest-1"), []byte("attest-2")
	s1, s2 := k1.Sign(m1), k2.Sign(m2)

	if len(s1) != SignatureSize {
		t.Fatalf("signature size = %d, want %d (G1)", len(s1), SignatureSize)
	}
	if !k1.Public().Verify(m1, s1) || k2.Public().Verify(m1, s1) {
		t.Fatal("single-sig verify wrong")
	}
	agg, err := Aggregate([][]byte{s1, s2})
	if err != nil {
		t.Fatal(err)
	}
	if len(agg) != SignatureSize {
		t.Fatalf("aggregate size = %d, want %d", len(agg), SignatureSize)
	}
	if !VerifyAggregate([]*NodePublicKey{k1.Public(), k2.Public()}, [][]byte{m1, m2}, agg) {
		t.Fatal("aggregate verify failed")
	}
	if VerifyAggregate([]*NodePublicKey{k1.Public(), k2.Public()}, [][]byte{m2, m1}, agg) {
		t.Fatal("aggregate must be bound to the message order")
	}
}

func TestNodeKeyMarshalRoundTrip(t *testing.T) {
	k, _ := GenerateNodeKey()
	raw, _ := k.MarshalBinary()
	k2, err := ParseNodePrivateKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("m")
	if !k2.Public().Verify(msg, k.Sign(msg)) {
		t.Fatal("parsed key produced a different key")
	}
	pubRaw, _ := k.Public().MarshalBinary()
	if _, err := ParseNodePublicKey(pubRaw); err != nil {
		t.Fatalf("parse public: %v", err)
	}
}

func TestCAIssueVerify(t *testing.T) {
	ca, _ := GenerateCA()
	key, _ := GenerateNodeKey()
	pop, _ := ProofOfPossession(key, "node-a")
	cert, err := ca.Issue("node-a", key.Public(), pop, 0)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ca.Public().Verify(cert, 0); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Wrong PoP is rejected.
	other, _ := GenerateNodeKey()
	badPop, _ := ProofOfPossession(other, "node-a")
	if _, err := ca.Issue("node-a", key.Public(), badPop, 0); err == nil {
		t.Fatal("PoP from a different key must be rejected")
	}
	// A different CA does not vouch for this cert.
	otherCA, _ := GenerateCA()
	if _, err := otherCA.Public().Verify(cert, 0); err == nil {
		t.Fatal("foreign CA must reject the cert")
	}
	// Expiry.
	expired, _ := ca.Issue("node-a", key.Public(), pop, 100)
	if _, err := ca.Public().Verify(expired, 200); err == nil {
		t.Fatal("expired cert must be rejected")
	}
	// Cert round-trips through the codec.
	raw, _ := EncodeCert(cert)
	got, err := DecodeCert(raw)
	if err != nil || got.NodeID != "node-a" || !bytes.Equal(got.CASig, cert.CASig) {
		t.Fatalf("cert codec round trip: %+v err=%v", got, err)
	}
}

// The showpiece: a 3-node cluster co-signs a checkpoint; the aggregate is a
// quorum certificate that verifies offline against the CA — and fails on any
// tamper, a missing signer, a foreign CA, or an unmet threshold.
func TestQuorumCertificate(t *testing.T) {
	ca, _ := GenerateCA()
	nodes := []string{"node-0", "node-1", "node-2"}
	keys := map[string]*NodePrivateKey{}
	certs := map[string]*NodeCert{}
	for _, id := range nodes {
		k, _ := GenerateNodeKey()
		keys[id] = k
		pop, _ := ProofOfPossession(k, id)
		certs[id], _ = ca.Issue(id, k.Public(), pop, 0)
	}

	ckpt := &Checkpoint{Height: 42, Term: 3, Digest: bytes.Repeat([]byte{0xab}, 32), Unix: 1_700_000_000}

	// Two of three nodes sign (a majority of 3).
	sigs := map[string][]byte{
		"node-0": SignCheckpoint(keys["node-0"], "node-0", ckpt),
		"node-1": SignCheckpoint(keys["node-1"], "node-1", ckpt),
	}
	qc, err := BuildQuorumCertificate(ckpt, sigs)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(qc.AggSig) != SignatureSize {
		t.Fatalf("quorum cert aggregate is %d bytes, want %d", len(qc.AggSig), SignatureSize)
	}

	if err := VerifyQuorum(ca.Public(), qc, certs, 2, 0); err != nil {
		t.Fatalf("valid quorum must verify: %v", err)
	}
	// Threshold not met.
	if err := VerifyQuorum(ca.Public(), qc, certs, 3, 0); err == nil {
		t.Fatal("2 signers must fail a threshold of 3")
	}
	// Tampered checkpoint (digest changed after signing).
	tampered, _ := DecodeQuorum(mustEncode(t, qc))
	tampered.Checkpoint.Digest[0] ^= 0xff
	if err := VerifyQuorum(ca.Public(), tampered, certs, 2, 0); err == nil {
		t.Fatal("a tampered checkpoint must fail")
	}
	// A signer with no cert.
	noCert := map[string]*NodeCert{"node-0": certs["node-0"]}
	if err := VerifyQuorum(ca.Public(), qc, noCert, 2, 0); err == nil {
		t.Fatal("a signer without a cert must fail")
	}
	// A foreign CA.
	otherCA, _ := GenerateCA()
	if err := VerifyQuorum(otherCA.Public(), qc, certs, 2, 0); err == nil {
		t.Fatal("a foreign CA must fail")
	}
	// A padded/duplicate signer list must not inflate the count.
	dup := &QuorumCertificate{Checkpoint: qc.Checkpoint, Signers: []string{"node-0", "node-0"}, AggSig: qc.AggSig}
	if err := VerifyQuorum(ca.Public(), dup, certs, 2, 0); err == nil {
		t.Fatal("duplicate signers must be rejected")
	}
	// Quorum cert round-trips through the codec and still verifies.
	rt, _ := DecodeQuorum(mustEncode(t, qc))
	if err := VerifyQuorum(ca.Public(), rt, certs, 2, 0); err != nil {
		t.Fatalf("round-tripped quorum must verify: %v", err)
	}
}

func mustEncode(t *testing.T, qc *QuorumCertificate) []byte {
	t.Helper()
	raw, err := EncodeQuorum(qc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
