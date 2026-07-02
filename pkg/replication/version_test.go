/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"testing"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

// applyPut drives a Put command through the FSM at the given log index and
// fails the test if the apply did not report success.
func applyPut(t *testing.T, fsm *FSM, index uint64, req *pb.RecordRequest) *pb.Status {
	t.Helper()
	data, err := encodeCommand(opPut, req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	res, ok := fsm.Apply(&raft.Log{Index: index, Data: data}).(*fsmResult)
	if !ok || res.err != nil {
		t.Fatalf("apply at %d: %#v", index, res)
	}
	return res.status
}

func versionOf(t *testing.T, kv server.KeyValueStorage, key *pb.Key) uint64 {
	t.Helper()
	rec, err := kv.Get(&pb.KeyRequest{Key: key})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Head == nil {
		t.Fatalf("record not found for %v", key)
	}
	return rec.Head.Version
}

// The reported version must equal the raft log index that wrote the record —
// the replica-independent version the whole design hangs on.
func TestVersionEqualsRaftIndex(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	key := sampleKey()

	applyPut(t, fsm, 7, &pb.RecordRequest{Key: key, Value: []byte("a")})
	if v := versionOf(t, kv, key); v != 7 {
		t.Fatalf("version after write at index 7 = %d, want 7", v)
	}

	applyPut(t, fsm, 12, &pb.RecordRequest{Key: key, Value: []byte("b")})
	if v := versionOf(t, kv, key); v != 12 {
		t.Fatalf("version after write at index 12 = %d, want 12", v)
	}
}

// Two independent replicas applying the same log entries (in index order, as raft
// guarantees) must land the same version. Replica B is given extra local history
// on unrelated keys first, so badger's per-DB commit timestamps differ between the
// nodes — if the version came from item.Version() the two would disagree; because
// it comes from the raft index in the envelope, they match.
func TestVersionDeterministicAcrossReplicas(t *testing.T) {
	key := sampleKey()
	a := newStorage(t)
	b := newStorage(t)
	fsmA := &FSM{Storage: a, Log: zap.NewNop()}
	fsmB := &FSM{Storage: b, Log: zap.NewNop()}

	// B has done extra local writes to another key, advancing badger's internal
	// commit-timestamp counter differently from A.
	other := &pb.Key{MajorKey: []byte("z"), RegionName: []byte("r"), MinorKey: []byte("m")}
	for i := uint64(1); i <= 4; i++ {
		applyPut(t, fsmB, i, &pb.RecordRequest{Key: other, Value: []byte("x")})
	}

	// The shared log for key, applied in the same increasing index order to both.
	for _, idx := range []uint64{10, 20, 30} {
		applyPut(t, fsmA, idx, &pb.RecordRequest{Key: key, Value: []byte("v")})
		applyPut(t, fsmB, idx, &pb.RecordRequest{Key: key, Value: []byte("v")})
	}

	va := versionOf(t, a, key)
	vb := versionOf(t, b, key)
	if va != vb {
		t.Fatalf("replica versions diverged: A=%d B=%d", va, vb)
	}
	if va != 30 {
		t.Fatalf("final version = %d, want 30 (last index)", va)
	}
}

// CAS must compare the caller's expected version against the stored ENVELOPE
// version (the raft index), not badger's item.Version().
func TestCompareAndSetUsesEnvelopeVersion(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	key := sampleKey()

	// Plain write at index 3 -> version 3.
	applyPut(t, fsm, 3, &pb.RecordRequest{Key: key, Value: []byte("v1")})

	// CAS with the correct expected version (3) at index 4 -> succeeds, bumps to 4.
	st := applyPut(t, fsm, 4, &pb.RecordRequest{
		Key: key, Value: []byte("v2"), CompareAndSet: true, Version: 3,
	})
	if !st.Updated {
		t.Fatal("CAS with correct version should succeed")
	}
	if v := versionOf(t, kv, key); v != 4 {
		t.Fatalf("version after CAS = %d, want 4", v)
	}

	// Replaying the now-stale version (3) must fail and leave state untouched.
	st = applyPut(t, fsm, 5, &pb.RecordRequest{
		Key: key, Value: []byte("v3"), CompareAndSet: true, Version: 3,
	})
	if st.Updated {
		t.Fatal("CAS with stale version should fail")
	}

	// putIfAbsent (expected version 0) on an existing key must fail.
	st = applyPut(t, fsm, 6, &pb.RecordRequest{
		Key: key, Value: []byte("v4"), CompareAndSet: true, Version: 0,
	})
	if st.Updated {
		t.Fatal("putIfAbsent on existing key should fail")
	}

	// The winning value is still v2 at version 4.
	rec, _ := kv.Get(&pb.KeyRequest{Key: key})
	if string(rec.Value) != "v2" || rec.Head.Version != 4 {
		t.Fatalf("final = %q@%d, want v2@4", rec.Value, rec.Head.Version)
	}
}
