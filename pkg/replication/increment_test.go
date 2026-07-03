/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"encoding/binary"
	"testing"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

func applyIncrement(t *testing.T, fsm *FSM, index uint64, req *pb.IncrementRequest) *pb.IncrementResponse {
	t.Helper()
	data, err := encodeCommand(opIncrement, req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	res, ok := fsm.Apply(&raft.Log{Index: index, Data: data}).(*fsmResult)
	if !ok || res.err != nil {
		t.Fatalf("increment apply at %d: %#v", index, res)
	}
	return res.incr
}

func applyBatch(t *testing.T, fsm *FSM, index uint64, req *pb.BatchRequest) *pb.Status {
	t.Helper()
	data, err := encodeCommand(opBatch, req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	res, ok := fsm.Apply(&raft.Log{Index: index, Data: data}).(*fsmResult)
	if !ok || res.err != nil {
		t.Fatalf("batch apply at %d: %#v", index, res)
	}
	return res.status
}

func counterValue(t *testing.T, kv server.KeyValueStorage, key *pb.Key) int64 {
	t.Helper()
	rec, err := kv.Get(&pb.KeyRequest{Key: key})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rec.Value) < 8 {
		t.Fatalf("counter payload too short: %d bytes", len(rec.Value))
	}
	return int64(binary.BigEndian.Uint64(rec.Value))
}

// Increment returns the previous value, persists the new one, and stamps the
// raft index as the version — the same envelope-versioning as Put.
func TestIncrement(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	key := sampleKey()

	// Absent key: starts at Initial=10, +5 -> prev 10, current 15, version 3.
	r := applyIncrement(t, fsm, 3, &pb.IncrementRequest{Key: key, Initial: 10, Delta: 5})
	if r.Previous != 10 || r.Current != 15 || r.Version != 3 {
		t.Fatalf("first increment = %+v, want prev 10 current 15 version 3", r)
	}

	// Existing key: Initial ignored, 15 +5 -> prev 15, current 20, version 7.
	r = applyIncrement(t, fsm, 7, &pb.IncrementRequest{Key: key, Initial: 10, Delta: 5})
	if r.Previous != 15 || r.Current != 20 || r.Version != 7 {
		t.Fatalf("second increment = %+v, want prev 15 current 20 version 7", r)
	}

	if v := counterValue(t, kv, key); v != 20 {
		t.Fatalf("stored counter = %d, want 20", v)
	}
	if v := versionOf(t, kv, key); v != 7 {
		t.Fatalf("stored version = %d, want 7", v)
	}

	// Negative delta works too.
	r = applyIncrement(t, fsm, 9, &pb.IncrementRequest{Key: key, Delta: -8})
	if r.Previous != 20 || r.Current != 12 {
		t.Fatalf("negative increment = %+v, want prev 20 current 12", r)
	}
}

// The counter and its version must be identical on two replicas applying the
// same log, regardless of each node's local badger history.
func TestIncrementDeterministicAcrossReplicas(t *testing.T) {
	key := sampleKey()
	a := newStorage(t)
	b := newStorage(t)
	fsmA := &FSM{Storage: a, Log: zap.NewNop()}
	fsmB := &FSM{Storage: b, Log: zap.NewNop()}

	// B carries extra unrelated local history.
	other := &pb.Key{MajorKey: []byte("z"), RegionName: []byte("r"), MinorKey: []byte("m")}
	for i := uint64(1); i <= 3; i++ {
		applyPut(t, fsmB, i, &pb.RecordRequest{Key: other, Value: []byte("x")})
	}

	for _, idx := range []uint64{10, 20, 30} {
		applyIncrement(t, fsmA, idx, &pb.IncrementRequest{Key: key, Delta: 2})
		applyIncrement(t, fsmB, idx, &pb.IncrementRequest{Key: key, Delta: 2})
	}

	if va, vb := counterValue(t, a, key), counterValue(t, b, key); va != vb || va != 6 {
		t.Fatalf("counter diverged: A=%d B=%d (want 6)", va, vb)
	}
	if va, vb := versionOf(t, a, key), versionOf(t, b, key); va != vb || va != 30 {
		t.Fatalf("version diverged: A=%d B=%d (want 30)", va, vb)
	}
}

// A batch writes every record atomically; each entry carries the log index as
// its envelope version.
func TestBatch(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}

	k1 := &pb.Key{MajorKey: []byte("t"), RegionName: []byte("R"), MinorKey: []byte("a")}
	k2 := &pb.Key{MajorKey: []byte("t"), RegionName: []byte("R"), MinorKey: []byte("b")}

	st := applyBatch(t, fsm, 42, &pb.BatchRequest{Records: []*pb.RecordRequest{
		{Key: k1, Value: []byte("one")},
		{Key: k2, Value: []byte("two")},
	}})
	if !st.Updated {
		t.Fatal("batch should report updated")
	}

	for _, tc := range []struct {
		key  *pb.Key
		want string
	}{{k1, "one"}, {k2, "two"}} {
		rec, err := kv.Get(&pb.KeyRequest{Key: tc.key})
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if string(rec.Value) != tc.want {
			t.Fatalf("value = %q, want %q", rec.Value, tc.want)
		}
		if rec.Head.Version != 42 {
			t.Fatalf("version = %d, want 42 (log index)", rec.Head.Version)
		}
	}
}
