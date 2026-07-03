/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"bytes"
	"testing"

	"go.arpabet.com/consensusdb/pkg/pb"
)

// The raft log stores value-packed command payloads. Two properties must hold for
// the replicated state machine to stay consistent:
//  1. determinism — the same command packs to byte-identical output every time (so
//     every replica writes the same log entry);
//  2. round-trip fidelity — decode(encode(x)) == x for every command shape,
//     including nested Key, slices of records, and empty/nil fields.
func TestCommandEncodingDeterministicRoundTrip(t *testing.T) {
	key := &pb.Key{
		MajorKey:   []byte("acme"),
		RegionName: []byte("USERS"),
		MinorKey:   []byte("alice"),
		Timestamp:  &pb.TimeUUID{MostSigBits: 123, LeastSigBits: 456},
	}

	cases := []struct {
		name string
		op   opCode
		msg  interface{}
	}{
		{"put", opPut, &pb.RecordRequest{
			Key: key, Value: []byte("v"), Metadata: 7, TtlSeconds: 60,
			CompareAndSet: true, Version: 9, ExpiresAt: 1000,
		}},
		{"touch", opTouch, &pb.RecordRequest{Key: key, TtlSeconds: 30, ExpiresAt: 2000}},
		{"remove", opRemove, &pb.KeyRequest{Key: key, HeadOnly: true}},
		{"increment", opIncrement, &pb.IncrementRequest{
			Key: key, Initial: 10, Delta: -3, TtlSeconds: 5, ExpiresAt: 3000,
		}},
		{"batch", opBatch, &pb.BatchRequest{Records: []*pb.RecordRequest{
			{Key: key, Value: []byte("a")},
			{Key: &pb.Key{MinorKey: []byte("bob")}, Value: nil}, // nil Timestamp + nil value
		}}},
		{"reclaim", opReclaim, &pb.ReclaimRequest{Entries: []*pb.ReclaimEntry{
			{EntryKey: []byte("k1"), Version: 1},
			{EntryKey: []byte("k2"), Version: 2},
		}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b1, err := encodeCommand(tc.op, tc.msg)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			// Determinism: repeated packing of the same command is byte-identical
			// (so every replica writes the same raft log entry).
			for i := 0; i < 3; i++ {
				bn, err := encodeCommand(tc.op, tc.msg)
				if err != nil {
					t.Fatalf("re-encode: %v", err)
				}
				if !bytes.Equal(b1, bn) {
					t.Fatalf("non-deterministic encoding on attempt %d", i)
				}
			}
			// Round-trip stability: decoding then re-encoding yields the identical
			// bytes, so the FSM's view is invariant under the raft log format.
			op, got, err := decodeCommand(b1)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if op != tc.op {
				t.Fatalf("op = %d, want %d", op, tc.op)
			}
			b2, err := encodeCommand(op, got)
			if err != nil {
				t.Fatalf("re-encode decoded: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Fatalf("round-trip not byte-stable:\n first  %x\n second %x", b1, b2)
			}
		})
	}
}

// Content fidelity: every meaningful field of a rich put command survives the
// encode/decode round-trip (guards against silent data loss in the value packing).
func TestCommandRoundTripPreservesFields(t *testing.T) {
	orig := &pb.RecordRequest{
		Key: &pb.Key{
			MajorKey:   []byte("acme"),
			RegionName: []byte("USERS"),
			MinorKey:   []byte("alice"),
			Timestamp:  &pb.TimeUUID{MostSigBits: 123, LeastSigBits: 456},
		},
		Value: []byte("balance=100"), Metadata: 7, TtlSeconds: 60,
		CompareAndSet: true, Version: 9, Timeout: 100, ExpiresAt: 1700000000,
	}
	_, decoded, err := decodeCommand(mustEncode(t, opPut, orig))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := decoded.(*pb.RecordRequest)

	if string(got.Value) != "balance=100" || got.Metadata != 7 || got.TtlSeconds != 60 ||
		!got.CompareAndSet || got.Version != 9 || got.Timeout != 100 || got.ExpiresAt != 1700000000 {
		t.Fatalf("scalar fields lost: %+v", got)
	}
	if got.Key == nil || string(got.Key.MajorKey) != "acme" ||
		string(got.Key.RegionName) != "USERS" || string(got.Key.MinorKey) != "alice" {
		t.Fatalf("key fields lost: %+v", got.Key)
	}
	if got.Key.Timestamp == nil || got.Key.Timestamp.MostSigBits != 123 || got.Key.Timestamp.LeastSigBits != 456 {
		t.Fatalf("nested timestamp lost: %+v", got.Key.Timestamp)
	}
}

func mustEncode(t *testing.T, op opCode, msg interface{}) []byte {
	t.Helper()
	b, err := encodeCommand(op, msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}
