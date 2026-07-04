/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package verify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

// A store whose FSM applied a few entries, then a full backup of it, then an
// offline verification of that backup against a quorum certificate of its head.
func TestVerifyBackupAgainstQuorum(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// 1. Open storage and simulate the FSM apply path: advance the hash chain and
	//    persist its head exactly as pkg/replication's FSM does.
	conf := &server.Configuration{DataDir: dir, FileIO: true}
	if err := conf.PostConstruct(); err != nil {
		t.Fatal(err)
	}
	storage, err := server.OpenKeyValueStorage(conf, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	chain := ledger.NewHashChain(0, ledger.GenesisDigest)
	for i := 1; i <= 5; i++ {
		cmd := []byte{byte(i), 'p', 'u', 't'}
		// store a real record so the dump is non-trivial
		key := &pb.Key{MajorKey: []byte("acme"), RegionName: []byte("R"), MinorKey: []byte{byte(i)}}
		if _, err := storage.Put(&pb.RecordRequest{Key: key, Value: cmd}, uint64(i)); err != nil {
			t.Fatal(err)
		}
		chain.Advance(uint64(i), 2, cmd)
		idx, digest := chain.Head()
		ledger.StoreHead(storage, idx, digest, uint64(i))
	}
	wantIndex, wantDigest := chain.Head()

	// 2. Back up the store to an (encrypted) file.
	dump := filepath.Join(t.TempDir(), "cluster.dump")
	f, err := os.Create(dump)
	if err != nil {
		t.Fatal(err)
	}
	const password = "verify-pass"
	container, err := backup.NewWriter(f, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Backup(container, 0); err != nil {
		t.Fatal(err)
	}
	container.Close()
	f.Close()
	storage.Close()

	// 3. Build a quorum certificate for the chain head.
	ca, _ := ledger.GenerateCA()
	ids := []string{"node-0", "node-1", "node-2"}
	certs := map[string][]byte{}
	sigs := map[string][]byte{}
	ckpt := &ledger.Checkpoint{Height: wantIndex, Term: 2, Digest: wantDigest[:], Unix: 1}
	for _, id := range ids[:2] { // a 2-of-3 quorum
		k, _ := ledger.GenerateNodeKey()
		pop, _ := ledger.ProofOfPossession(k, id)
		cert, _ := ca.Issue(id, k.Public(), pop, 0)
		raw, _ := ledger.EncodeCert(cert)
		certs[id] = raw
		sigs[id] = ledger.SignCheckpoint(k, id, ckpt)
	}
	qc, err := ledger.BuildQuorumCertificate(ckpt, sigs)
	if err != nil {
		t.Fatal(err)
	}
	qcBytes, _ := ledger.EncodeQuorum(qc)
	caPub, _ := ca.Public().MarshalBinary()

	opts := Options{
		Source: dump, Password: password,
		CACert: caPub, QuorumCert: qcBytes,
		NodeCerts: [][]byte{certs["node-0"], certs["node-1"]},
		Threshold: 2,
	}

	// 4. Verify — the backup matches the quorum-certified checkpoint, with
	//    monotonic progress reported.
	var lastPct int
	res, err := VerifyBackup(ctx, opts, func(pct int) {
		if pct < lastPct {
			t.Errorf("progress went backwards: %d after %d", pct, lastPct)
		}
		lastPct = pct
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified, got: %s", res.Message)
	}
	if res.Height != wantIndex || res.Digest != ledger.DigestHex(wantDigest) {
		t.Fatalf("verified head = %d/%s, want %d/%s", res.Height, res.Digest, wantIndex, ledger.DigestHex(wantDigest))
	}
	if lastPct != 100 {
		t.Fatalf("final progress = %d, want 100", lastPct)
	}

	// 5. A quorum certificate for a DIFFERENT digest must not verify against this
	//    backup (the backup's state doesn't match that checkpoint).
	badDigest := make([]byte, 32)
	badCkpt := &ledger.Checkpoint{Height: wantIndex, Term: 2, Digest: badDigest, Unix: 1}
	badSigs := map[string][]byte{}
	// reissue keys for the bad checkpoint
	badCerts := map[string][]byte{}
	for _, id := range ids[:2] {
		k, _ := ledger.GenerateNodeKey()
		pop, _ := ledger.ProofOfPossession(k, id)
		cert, _ := ca.Issue(id, k.Public(), pop, 0)
		raw, _ := ledger.EncodeCert(cert)
		badCerts[id] = raw
		badSigs[id] = ledger.SignCheckpoint(k, id, badCkpt)
	}
	badQC, _ := ledger.BuildQuorumCertificate(badCkpt, badSigs)
	badQCBytes, _ := ledger.EncodeQuorum(badQC)
	res, err = VerifyBackup(ctx, Options{
		Source: dump, Password: password, CACert: caPub, QuorumCert: badQCBytes,
		NodeCerts: [][]byte{badCerts["node-0"], badCerts["node-1"]}, Threshold: 2,
	}, nil)
	if err != nil {
		t.Fatalf("verify(bad): %v", err)
	}
	if res.Verified {
		t.Fatal("a quorum certificate for a different digest must not verify against this backup")
	}
	if !bytes.Contains([]byte(res.Message), []byte("does not match")) {
		t.Fatalf("unexpected mismatch message: %s", res.Message)
	}
}
