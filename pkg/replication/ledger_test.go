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

// A coordinator collects aggregatable attestations by asking every node to
// co-sign ONE canonical checkpoint (VerifyQuorum reconstructs each signer's
// message from a single checkpoint, so per-node timestamps cannot aggregate).
// Attest signs the requested checkpoint only when it matches this node's own
// derived head, refuses (unsigned, reporting the actual head) when it does not,
// and stamps a standalone statement when nothing specific is requested.
func TestLedgerAttestCoSign(t *testing.T) {
	fsm := &FSM{Storage: newStorage(t), Log: zap.NewNop()}
	key := &pb.Key{MajorKey: []byte("acme"), RegionName: []byte("LEDGER"), MinorKey: []byte("balance")}
	data, err := encodeCommand(opPut, &pb.RecordRequest{Key: key, Value: []byte("100")})
	if err != nil {
		t.Fatal(err)
	}
	if res, ok := fsm.Apply(&raft.Log{Index: 1, Term: 2, Data: data}).(*fsmResult); !ok || res.err != nil {
		t.Fatalf("apply: %#v", res)
	}

	ca, err := ledger.GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	blsKey, err := ledger.GenerateNodeKey()
	if err != nil {
		t.Fatal(err)
	}
	pop, _ := ledger.ProofOfPossession(blsKey, "node-a")
	cert, err := ca.Issue("node-a", blsKey.Public(), pop, 0)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ledger.NewNodeSigner(blsKey, cert)
	if err != nil {
		t.Fatal(err)
	}
	svc := &LedgerService{FSM: fsm, Log: zap.NewNop(), signer: signer}

	// Standalone: the node's own head, signed with its own stamp.
	own, nodeID, sig, certRaw, signed := svc.Attest(nil)
	if !signed || nodeID != "node-a" || len(sig) == 0 || len(certRaw) == 0 || own.Height != 1 {
		t.Fatalf("standalone attest = (%v %q h=%d), want signed node-a at height 1", signed, nodeID, own.Height)
	}

	// Co-sign: the coordinator's exact canonical bytes (its unix stamp) are
	// signed, and the single-signer certificate verifies against the CA.
	want := &ledger.Checkpoint{Height: own.Height, Digest: own.Digest, Unix: own.Unix + 100}
	got, nodeID, sig, _, signed := svc.Attest(want)
	if !signed || got.Unix != want.Unix {
		t.Fatalf("co-sign attest = (%v unix=%d), want signed with the requested unix %d", signed, got.Unix, want.Unix)
	}
	qc, err := ledger.BuildQuorumCertificate(want, map[string][]byte{nodeID: sig})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.VerifyQuorum(ca.Public(), qc, map[string]*ledger.NodeCert{"node-a": cert}, 1, 0); err != nil {
		t.Fatalf("co-signed certificate must verify: %v", err)
	}

	// A requested head that is not this node's own is refused, actual head reported.
	bad := &ledger.Checkpoint{Height: own.Height + 5, Digest: own.Digest, Unix: own.Unix}
	got, _, _, _, signed = svc.Attest(bad)
	if signed || got.Height != own.Height {
		t.Fatalf("mismatched attest = (%v h=%d), want unsigned with own height %d", signed, got.Height, own.Height)
	}
}
