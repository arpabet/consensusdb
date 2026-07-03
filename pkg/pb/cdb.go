/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

// Package pb holds the consensusdb data model. These are plain Go structs encoded
// with the go.arpabet.com/value framework (canonical, deterministic packing), not
// protobuf: raft commands are serialized with value.PackStruct (see
// pkg/replication/command.go) and the value-rpc data plane hand-maps them to
// value.Map (see pkg/server/vrpc_data.go). The `value:"…"` tags name each field
// in the packed form; keep them stable — they are on-disk in the raft log.
//
// Field names and types mirror the retired proto/cdb.proto so the rest of the
// codebase is unchanged.
package pb

// ChangeType classifies a WatchEvent.
type ChangeType int32

const (
	ChangeType_WATCH_SET    ChangeType = 0 // key created or updated
	ChangeType_WATCH_DELETE ChangeType = 1 // key removed (or reclaimed after expiry)
)

// RangeType selects the range scan direction/semantics.
type RangeType int32

const (
	RangeType_LESS_OR_EQUAL RangeType = 0
)

// TimeUUID is a 128-bit time-based identifier (Java-compatible split).
type TimeUUID struct {
	MostSigBits  int64 `value:"mostSigBits"`
	LeastSigBits int64 `value:"leastSigBits"`
}

// Key is the three-level record identity plus an optional timestamp.
type Key struct {
	MajorKey   []byte    `value:"majorKey"`   // partition key (empty = replicated in all partitions)
	RegionName []byte    `value:"regionName"` // logical table within the partition
	MinorKey   []byte    `value:"minorKey"`   // record key within the partition
	Timestamp  *TimeUUID `value:"timestamp"`  // TimeUUID if present
}

type KeyRequest struct {
	Key      *Key  `value:"key"`
	HeadOnly bool  `value:"headOnly"`
	Timeout  int32 `value:"timeout"` // SLA of the operation if not 0
}

type RecordRequest struct {
	Key           *Key   `value:"key"`
	Value         []byte `value:"value"`
	Metadata      int32  `value:"metadata"`      // for user 8 bits only, 9 bit deletedBit
	TtlSeconds    int64  `value:"ttlSeconds"`    // client input: relative TTL in seconds, 0 = none
	CompareAndSet bool   `value:"compareAndSet"`
	Version       uint64 `value:"version"` // if 0 then putIfAbsent
	Timeout       int32  `value:"timeout"` // SLA of the operation if not 0
	// Absolute expiry (unix seconds) computed once on the leader from ttlSeconds
	// before the write enters the raft log, so every replica stores the same expiry.
	ExpiresAt int64 `value:"expiresAt"`
}

type IncrementRequest struct {
	Key        *Key  `value:"key"`
	Initial    int64 `value:"initial"`    // counter start value when the key is absent
	Delta      int64 `value:"delta"`      // amount to add (may be negative)
	TtlSeconds int64 `value:"ttlSeconds"` // client input: relative TTL in seconds, 0 = none
	Timeout    int32 `value:"timeout"`    // SLA of the operation if not 0
	ExpiresAt  int64 `value:"expiresAt"`  // absolute expiry computed on the leader
}

type IncrementResponse struct {
	Previous int64  `value:"previous"` // counter value before delta was applied
	Current  int64  `value:"current"`  // counter value after delta was applied
	Version  uint64 `value:"version"`  // envelope version (raft log index on the replicated path)
}

type BatchRequest struct {
	Records []*RecordRequest `value:"records"` // written all-or-nothing in one engine txn
	Timeout int32            `value:"timeout"` // SLA of the operation if not 0
}

// ReclaimEntry is one expired key the leader proposes to delete; the FSM removes
// it only if its stored envelope version still matches (deterministic reclaim).
type ReclaimEntry struct {
	EntryKey []byte `value:"entryKey"`
	Version  uint64 `value:"version"`
}

type ReclaimRequest struct {
	Entries []*ReclaimEntry `value:"entries"`
}

type WatchRequest struct {
	Prefix  *Key  `value:"prefix"`  // watch keys under this prefix; empty prefix watches everything
	Timeout int32 `value:"timeout"` // SLA of the operation if not 0
}

type WatchEvent struct {
	Key     *Key       `value:"key"`
	Value   []byte     `value:"value"` // empty for WATCH_DELETE
	Version uint64     `value:"version"`
	Type    ChangeType `value:"type"`
}

type RangeRequest struct {
	Key        *Key      `value:"key"`
	HeadOnly   bool      `value:"headOnly"`
	Type       RangeType `value:"type"`
	NumRecords int32     `value:"numRecords"`
	Timeout    int32     `value:"timeout"`
}

type ScanRequest struct {
	HeadOnly bool `value:"headOnly"`
}

type EnumerateRequest struct {
	Prefix  *Key  `value:"prefix"`  // region/prefix to scan; empty scans everything
	Ordered bool  `value:"ordered"` // server sorts results by the decoded minor key (lexical)
	Reverse bool  `value:"reverse"` // with ordered, stream in descending order
	Timeout int32 `value:"timeout"`
}

type Status struct {
	Updated bool `value:"updated"`
}

type Head struct {
	Version   uint64 `value:"version"`
	ExpiresAt uint64 `value:"expiresAt"`
	DiskSize  int64  `value:"diskSize"`
	Metadata  int32  `value:"metadata"` // for user 8 bits only, 9 bit deletedBit
}

type Record struct {
	Key   *Key   `value:"key"`
	Head  *Head  `value:"head"` // if present, the record was found
	Value []byte `value:"value"`
}

type Block struct {
	Record []*Record `value:"record"`
}
