/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

// bufferSink is a raft.SnapshotSink that captures the persisted snapshot bytes.
type bufferSink struct{ buf bytes.Buffer }

func (s *bufferSink) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *bufferSink) Close() error                { return nil }
func (s *bufferSink) ID() string                  { return "test-snapshot" }
func (s *bufferSink) Cancel() error               { return nil }

func newStorage(t *testing.T) server.KeyValueStorage {
	t.Helper()
	conf := &server.Configuration{DataDir: t.TempDir(), FileIO: true}
	if err := conf.PostConstruct(); err != nil {
		t.Fatalf("conf: %v", err)
	}
	kv, err := server.OpenKeyValueStorage(conf, zap.NewNop())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = kv.Close() })
	return kv
}

func sampleKey() *pb.Key {
	return &pb.Key{
		MajorKey:   []byte("alex"),
		RegionName: []byte("accounts"),
		MinorKey:   []byte("Bank1"),
	}
}

func TestCommandCodec(t *testing.T) {
	rr := &pb.RecordRequest{Key: sampleKey(), Value: []byte("hello")}
	data, err := encodeCommand(opPut, rr)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	op, msg, err := decodeCommand(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if op != opPut {
		t.Fatalf("op = %d, want %d", op, opPut)
	}
	got, ok := msg.(*pb.RecordRequest)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if !bytes.Equal(got.Value, []byte("hello")) {
		t.Fatalf("value = %q", got.Value)
	}

	if _, _, err := decodeCommand([]byte{0xFF, 1, 2}); err == nil {
		t.Fatal("expected error for unknown op")
	}
	if _, _, err := decodeCommand(nil); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestFSMApply(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}

	// Put through the FSM.
	putData, _ := encodeCommand(opPut, &pb.RecordRequest{Key: sampleKey(), Value: []byte("v1")})
	res := fsm.Apply(&raft.Log{Index: 1, Data: putData})
	if r, ok := res.(*fsmResult); !ok || r.err != nil || !r.status.Updated {
		t.Fatalf("put apply result = %#v", res)
	}

	rec, err := kv.Get(&pb.KeyRequest{Key: sampleKey()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(rec.Value, []byte("v1")) {
		t.Fatalf("stored value = %q, want v1", rec.Value)
	}

	// Remove through the FSM.
	rmData, _ := encodeCommand(opRemove, &pb.KeyRequest{Key: sampleKey()})
	res = fsm.Apply(&raft.Log{Index: 2, Data: rmData})
	if r, ok := res.(*fsmResult); !ok || r.err != nil {
		t.Fatalf("remove apply result = %#v", res)
	}
	rec, err = kv.Get(&pb.KeyRequest{Key: sampleKey()})
	if err != nil {
		t.Fatalf("get after remove: %v", err)
	}
	if len(rec.Value) != 0 {
		t.Fatalf("value present after remove: %q", rec.Value)
	}
}

func TestFSMSnapshotRestore(t *testing.T) {
	src := newStorage(t)
	fsm := &FSM{Storage: src, Log: zap.NewNop()}
	putData, _ := encodeCommand(opPut, &pb.RecordRequest{Key: sampleKey(), Value: []byte("snap")})
	fsm.Apply(&raft.Log{Index: 1, Data: putData})

	// Persist a snapshot into a buffer via the sink shim.
	snap, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	sink := &bufferSink{}
	if err := snap.Persist(sink); err != nil {
		t.Fatalf("persist: %v", err)
	}

	// Restore into a fresh storage and verify the value is present.
	dst := newStorage(t)
	dstFSM := &FSM{Storage: dst, Log: zap.NewNop()}
	if err := dstFSM.Restore(io.NopCloser(bytes.NewReader(sink.buf.Bytes()))); err != nil {
		t.Fatalf("restore: %v", err)
	}
	rec, err := dst.Get(&pb.KeyRequest{Key: sampleKey()})
	if err != nil {
		t.Fatalf("get restored: %v", err)
	}
	if !bytes.Equal(rec.Value, []byte("snap")) {
		t.Fatalf("restored value = %q, want snap", rec.Value)
	}
}

// TestSingleNodeRaft exercises the full Apply path through a real (in-memory)
// raft instance: leader election, log replication, FSM apply, local read-back.
func TestSingleNodeRaft(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}

	store := raft.NewInmemStore()
	snaps := raft.NewInmemSnapshotStore()
	addr, transport := raft.NewInmemTransport("")

	config := raft.DefaultConfig()
	config.LocalID = "node-test"
	config.Logger = nil

	r, err := raft.NewRaft(config, fsm, store, store, snaps, transport)
	if err != nil {
		t.Fatalf("new raft: %v", err)
	}
	defer r.Shutdown()

	f := r.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{Suffrage: raft.Voter, ID: config.LocalID, Address: addr}},
	})
	if err := f.Error(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	waitLeader(t, r)

	putData, _ := encodeCommand(opPut, &pb.RecordRequest{Key: sampleKey(), Value: []byte("raftval")})
	af := r.Apply(putData, 5*time.Second)
	if err := af.Error(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res, ok := af.Response().(*fsmResult); !ok || res.err != nil || !res.status.Updated {
		t.Fatalf("apply response = %#v", af.Response())
	}

	rec, err := kv.Get(&pb.KeyRequest{Key: sampleKey()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(rec.Value, []byte("raftval")) {
		t.Fatalf("replicated value = %q, want raftval", rec.Value)
	}
}

func waitLeader(t *testing.T, r *raft.Raft) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.State() == raft.Leader {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("node did not become leader in time")
}
