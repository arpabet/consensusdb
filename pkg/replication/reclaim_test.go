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
	"go.arpabet.com/store"
	"go.uber.org/zap"
)

func pastExpiry() int64 { return time.Now().Unix() - 60 }

func applyReclaim(t *testing.T, fsm *FSM, index uint64, req *pb.ReclaimRequest) int {
	t.Helper()
	data, err := encodeCommand(opReclaim, req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	res, ok := fsm.Apply(&raft.Log{Index: index, Data: data}).(*fsmResult)
	if !ok || res.err != nil {
		t.Fatalf("reclaim apply at %d: %#v", index, res)
	}
	return res.reclaimed
}

// ScanExpired finds only expired entries; Reclaim removes them and leaves live
// entries intact.
func TestScanAndReclaimExpired(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	ctx := context.Background()

	live := sampleKey()
	expired := &pb.Key{MajorKey: []byte("alex"), RegionName: []byte("accounts"), MinorKey: []byte("old")}

	applyPut(t, fsm, 1, &pb.RecordRequest{Key: live, Value: []byte("live")})
	applyPut(t, fsm, 2, &pb.RecordRequest{Key: expired, Value: []byte("stale"), ExpiresAt: pastExpiry()})

	entries, err := kv.ScanExpired(ctx, 0)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("scan found %d expired, want 1", len(entries))
	}
	if entries[0].Version != 2 {
		t.Fatalf("expired entry version = %d, want 2", entries[0].Version)
	}

	n, err := kv.Reclaim(&pb.ReclaimRequest{Entries: entries})
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed %d, want 1", n)
	}

	// Physically gone: a second scan finds nothing.
	if again, _ := kv.ScanExpired(ctx, 0); len(again) != 0 {
		t.Fatalf("expired entry still present after reclaim: %d", len(again))
	}
	// Live entry untouched.
	if rec, _ := kv.Get(&pb.KeyRequest{Key: live}); rec.Head == nil || string(rec.Value) != "live" {
		t.Fatalf("live entry damaged: %+v", rec)
	}
}

// Reclaim must not delete an entry that was rewritten after discovery — the
// version guard is what makes concurrent refresh safe.
func TestReclaimVersionConditioned(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	ctx := context.Background()
	key := sampleKey()

	applyPut(t, fsm, 5, &pb.RecordRequest{Key: key, Value: []byte("stale"), ExpiresAt: pastExpiry()})
	entries, _ := kv.ScanExpired(ctx, 0) // {key, version 5}

	// The key is refreshed (new version, no expiry) before the reclaim applies.
	applyPut(t, fsm, 8, &pb.RecordRequest{Key: key, Value: []byte("fresh")})

	n, err := kv.Reclaim(&pb.ReclaimRequest{Entries: entries})
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 0 {
		t.Fatalf("reclaimed %d, want 0 (version changed)", n)
	}
	if rec, _ := kv.Get(&pb.KeyRequest{Key: key}); rec.Head == nil || string(rec.Value) != "fresh" {
		t.Fatalf("refreshed entry was wrongly reclaimed: %+v", rec)
	}
}

// Reclaiming through the FSM emits a WatchDelete, so expiry surfaces to watchers.
func TestReclaimEmitsWatchDelete(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	ctx := context.Background()
	key := sampleKey()

	applyPut(t, fsm, 3, &pb.RecordRequest{Key: key, Value: []byte("x"), ExpiresAt: pastExpiry()})
	entries, _ := kv.ScanExpired(ctx, 0)

	// Subscribe after the write so we only observe the delete.
	events, stop := startWatch(t, kv, nil)
	defer stop()

	if n := applyReclaim(t, fsm, 4, &pb.ReclaimRequest{Entries: entries}); n != 1 {
		t.Fatalf("reclaimed %d, want 1", n)
	}

	ev := waitEvent(t, events)
	if ev.Type != store.WatchDelete {
		t.Fatalf("event type = %v, want delete", ev.Type)
	}
}

// The same Reclaim command applied on two replicas removes the same entry on both
// — the version-conditioned delete is deterministic (no wall-clock at apply).
func TestReclaimDeterministicAcrossReplicas(t *testing.T) {
	a := newStorage(t)
	b := newStorage(t)
	fsmA := &FSM{Storage: a, Log: zap.NewNop()}
	fsmB := &FSM{Storage: b, Log: zap.NewNop()}
	ctx := context.Background()
	key := sampleKey()

	put := &pb.RecordRequest{Key: key, Value: []byte("stale"), ExpiresAt: pastExpiry()}
	applyPut(t, fsmA, 5, put)
	applyPut(t, fsmB, 5, put)

	// Discovery happens once (on the leader); the resulting command is applied to
	// both replicas.
	entries, _ := a.ScanExpired(ctx, 0)
	req := &pb.ReclaimRequest{Entries: entries}

	nA := applyReclaim(t, fsmA, 6, req)
	nB := applyReclaim(t, fsmB, 6, req)
	if nA != nB || nA != 1 {
		t.Fatalf("reclaim diverged: A=%d B=%d (want 1)", nA, nB)
	}
	if ea, _ := a.ScanExpired(ctx, 0); len(ea) != 0 {
		t.Fatalf("A still has expired entries: %d", len(ea))
	}
	if eb, _ := b.ScanExpired(ctx, 0); len(eb) != 0 {
		t.Fatalf("B still has expired entries: %d", len(eb))
	}
}
