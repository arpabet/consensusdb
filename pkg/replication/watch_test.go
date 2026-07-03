/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/store"
	"go.uber.org/zap"
)

// startWatch subscribes to prefix and returns a channel of events plus a stop
// func. It sleeps briefly so the watch goroutine registers before the caller
// triggers a mutation (subscribe is synchronous inside WatchRaw, but its
// goroutine still has to be scheduled first).
func startWatch(t *testing.T, kv server.KeyValueStorage, prefix []byte) (<-chan *store.WatchEvent, func()) {
	t.Helper()
	events := make(chan *store.WatchEvent, 16)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = kv.WatchRaw(ctx, prefix, func(ev *store.WatchEvent) bool {
			select {
			case events <- ev:
			case <-ctx.Done():
				return false
			}
			return true
		})
	}()
	time.Sleep(50 * time.Millisecond)
	return events, cancel
}

func waitEvent(t *testing.T, events <-chan *store.WatchEvent) *store.WatchEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch event")
		return nil
	}
}

func expectNoEvent(t *testing.T, events <-chan *store.WatchEvent) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("unexpected watch event: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

// A mutation applied through the FSM (the replicated commit path) must reach a
// local watcher — this is what makes cross-node watch work — and a remove must
// surface as a delete.
func TestWatchFromApplyPath(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	key := sampleKey()

	events, stop := startWatch(t, kv, nil) // watch everything
	defer stop()

	applyPut(t, fsm, 5, &pb.RecordRequest{Key: key, Value: []byte("hello")})

	ev := waitEvent(t, events)
	if ev.Type != store.WatchSet {
		t.Fatalf("event type = %v, want set", ev.Type)
	}
	if string(ev.Value) != "hello" {
		t.Fatalf("event value = %q, want hello", ev.Value)
	}
	if ev.Version != 5 {
		t.Fatalf("event version = %d, want 5 (log index)", ev.Version)
	}

	// Remove emits a delete.
	rmData, _ := encodeCommand(opRemove, &pb.KeyRequest{Key: key})
	if res, ok := fsm.Apply(&raft.Log{Index: 6, Data: rmData}).(*fsmResult); !ok || res.err != nil {
		t.Fatalf("remove apply: %#v", res)
	}
	ev = waitEvent(t, events)
	if ev.Type != store.WatchDelete {
		t.Fatalf("event type = %v, want delete", ev.Type)
	}
}

// A prefix watcher sees only changes to matching keys.
func TestWatchPrefixFilter(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}

	// Watch only keys whose major key is "alex".
	prefix, err := server.EncodeKeyPrefix(&pb.Key{MajorKey: []byte("alex")}, server.MajorKeyField)
	if err != nil {
		t.Fatalf("encode prefix: %v", err)
	}
	events, stop := startWatch(t, kv, prefix)
	defer stop()

	// Non-matching major key: no event.
	applyPut(t, fsm, 10, &pb.RecordRequest{
		Key:   &pb.Key{MajorKey: []byte("bob"), RegionName: []byte("accounts"), MinorKey: []byte("x")},
		Value: []byte("other"),
	})
	expectNoEvent(t, events)

	// Matching major key: delivered.
	applyPut(t, fsm, 11, &pb.RecordRequest{
		Key:   &pb.Key{MajorKey: []byte("alex"), RegionName: []byte("accounts"), MinorKey: []byte("x")},
		Value: []byte("mine"),
	})
	ev := waitEvent(t, events)
	if string(ev.Value) != "mine" || ev.Version != 11 {
		t.Fatalf("event = %q@%d, want mine@11", ev.Value, ev.Version)
	}
}
