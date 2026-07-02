/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.uber.org/zap"
)

// A command carries an absolute expiresAt (computed once on the leader), so two
// replicas applying it store the exact same expiry — not each its own now+ttl.
func TestExpiryDeterministicAcrossReplicas(t *testing.T) {
	key := sampleKey()
	a := newStorage(t)
	b := newStorage(t)
	fsmA := &FSM{Storage: a, Log: zap.NewNop()}
	fsmB := &FSM{Storage: b, Log: zap.NewNop()}

	expiresAt := time.Now().Unix() + 3600 // leader-computed absolute expiry
	req := &pb.RecordRequest{Key: key, Value: []byte("v"), ExpiresAt: expiresAt}

	applyPut(t, fsmA, 10, req)
	applyPut(t, fsmB, 10, req)

	recA, _ := a.Get(&pb.KeyRequest{Key: key})
	recB, _ := b.Get(&pb.KeyRequest{Key: key})
	if recA.Head.ExpiresAt != recB.Head.ExpiresAt {
		t.Fatalf("expiry diverged: A=%d B=%d", recA.Head.ExpiresAt, recB.Head.ExpiresAt)
	}
	if recA.Head.ExpiresAt != uint64(expiresAt) {
		t.Fatalf("stored expiry = %d, want %d", recA.Head.ExpiresAt, expiresAt)
	}
}

// An expired entry is hidden on read (lazy expiry) but is still physically
// present, so it deterministically blocks a putIfAbsent until a sweep removes it.
// This is the property that keeps CAS decisions identical across replicas: they
// never depend on a per-node wall clock.
func TestExpiredEntryHiddenButBlocksCAS(t *testing.T) {
	kv := newStorage(t)
	fsm := &FSM{Storage: kv, Log: zap.NewNop()}
	key := sampleKey()

	// Write already-expired (absolute expiry in the past).
	past := time.Now().Unix() - 60
	applyPut(t, fsm, 5, &pb.RecordRequest{Key: key, Value: []byte("stale"), ExpiresAt: past})

	// Read hides it.
	rec, err := kv.Get(&pb.KeyRequest{Key: key})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Head != nil {
		t.Fatalf("expired entry should be hidden on read, got head %+v", rec.Head)
	}

	// But putIfAbsent (CAS version 0) still fails: the envelope is physically
	// present, and existence checks use raw storage, not wall-clock expiry.
	st := applyPut(t, fsm, 6, &pb.RecordRequest{
		Key: key, Value: []byte("new"), CompareAndSet: true, Version: 0,
	})
	if st.Updated {
		t.Fatal("putIfAbsent over an expired-but-unswept key should fail")
	}

	// A non-expiring overwrite (no CAS) makes it visible again.
	applyPut(t, fsm, 7, &pb.RecordRequest{Key: key, Value: []byte("fresh")})
	rec, _ = kv.Get(&pb.KeyRequest{Key: key})
	if rec.Head == nil || string(rec.Value) != "fresh" {
		t.Fatalf("after overwrite = %+v / %q, want visible fresh", rec.Head, rec.Value)
	}
}
