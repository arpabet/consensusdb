/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"testing"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.uber.org/zap"
)

// The verifiable-ledger showpiece: three independent replicas apply the identical
// committed log and MUST converge to the same hash-chain head (a divergent head
// would be proof of tampering). A quorum then co-signs a checkpoint of that head,
// and the aggregate is a QuorumCertificate that verifies offline against the CA —
// the consensus made cryptographically visible.
func TestLedgerQuorumOverConvergedChain(t *testing.T) {
	nodeIDs := []string{"node-0", "node-1", "node-2"}

	// Build three replicas over independent storage and apply the same log.
	heads := make([][32]byte, len(nodeIDs))
	for n := range nodeIDs {
		fsm := &FSM{Storage: newStorage(t), Log: zap.NewNop()}
		mk := func(minor string) *pb.Key {
			return &pb.Key{MajorKey: []byte("acme"), RegionName: []byte("LEDGER"), MinorKey: []byte(minor)}
		}
		commands := []*pb.RecordRequest{
			{Key: mk("balance"), Value: []byte("100")},
			{Key: mk("balance"), Value: []byte("250")},
			{Key: mk("owner"), Value: []byte("alice")},
		}
		for i, cmd := range commands {
			data, err := encodeCommand(opPut, cmd)
			if err != nil {
				t.Fatal(err)
			}
			// term 2 for every entry; the same index/term/bytes on every replica.
			if res, ok := fsm.Apply(&raft.Log{Index: uint64(i + 1), Term: 2, Data: data}).(*fsmResult); !ok || res.err != nil {
				t.Fatalf("apply: %#v", res)
			}
		}
		idx, digest := fsm.ChainHead()
		if idx != uint64(len(commands)) {
			t.Fatalf("node %d head index = %d, want %d", n, idx, len(commands))
		}
		heads[n] = digest
	}

	// Convergence: all replicas derived the identical chain head.
	for n := 1; n < len(heads); n++ {
		if heads[n] != heads[0] {
			t.Fatalf("replica %d diverged: %s != %s", n, ledger.DigestHex(heads[n]), ledger.DigestHex(heads[0]))
		}
	}

	// Certify each node's BLS key under one CA.
	ca, err := ledger.GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]*ledger.NodePrivateKey{}
	certs := map[string]*ledger.NodeCert{}
	for _, id := range nodeIDs {
		k, err := ledger.GenerateNodeKey()
		if err != nil {
			t.Fatal(err)
		}
		pop, _ := ledger.ProofOfPossession(k, id)
		cert, err := ca.Issue(id, k.Public(), pop, 0)
		if err != nil {
			t.Fatal(err)
		}
		keys[id], certs[id] = k, cert
	}

	// A quorum (2 of 3) co-signs a checkpoint of the converged head.
	ckpt := &ledger.Checkpoint{Height: uint64(len(heads)), Term: 2, Digest: heads[0][:], Unix: 1_700_000_000}
	sigs := map[string][]byte{
		"node-0": ledger.SignCheckpoint(keys["node-0"], "node-0", ckpt),
		"node-1": ledger.SignCheckpoint(keys["node-1"], "node-1", ckpt),
	}
	qc, err := ledger.BuildQuorumCertificate(ckpt, sigs)
	if err != nil {
		t.Fatal(err)
	}
	if len(qc.AggSig) != ledger.SignatureSize {
		t.Fatalf("quorum certificate aggregate is %d bytes, want %d", len(qc.AggSig), ledger.SignatureSize)
	}

	// The certificate verifies offline against the CA at a majority threshold.
	if err := ledger.VerifyQuorum(ca.Public(), qc, certs, 2, 0); err != nil {
		t.Fatalf("quorum over the converged chain must verify: %v", err)
	}
	// And it fails the instant the attested digest is altered.
	qc.Checkpoint.Digest[0] ^= 0xff
	if err := ledger.VerifyQuorum(ca.Public(), qc, certs, 2, 0); err == nil {
		t.Fatal("a tampered checkpoint digest must fail verification")
	}
}
