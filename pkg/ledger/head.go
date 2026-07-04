/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/value"
)

/*
The hash-chain head is persisted as a single record in a reserved system tenant,
so it is captured by badger snapshots and reloaded after a restart — otherwise a
snapshot that truncated the raft log would restart the chain from genesis and
diverge from a node that had not snapshotted. It is written by the FSM apply path
on every committed entry.
*/

const (
	// HeadTenant / HeadRegion / HeadMinor address the persisted chain head. The
	// tenant is reserved (double-underscore) so it never collides with app data.
	HeadTenant = "__ledger"
	HeadRegion = "CHAIN"
	HeadMinor  = "head"
)

// headStore is the minimal storage surface the head persistence needs — satisfied
// by server.KeyValueStorage without a package import cycle.
type headStore interface {
	Get(*pb.KeyRequest) (*pb.Record, error)
	Put(*pb.RecordRequest, uint64) (*pb.Status, error)
}

func headKey() *pb.Key {
	return &pb.Key{MajorKey: []byte(HeadTenant), RegionName: []byte(HeadRegion), MinorKey: []byte(HeadMinor)}
}

type headRecord struct {
	Index  uint64 `value:"index"`
	Digest []byte `value:"digest"`
}

// LoadHead reads the persisted chain head, or genesis (0, zero) when absent.
func LoadHead(s headStore) (index uint64, digest [32]byte) {
	rec, err := s.Get(&pb.KeyRequest{Key: headKey()})
	if err != nil || rec == nil || len(rec.Value) == 0 {
		return 0, GenesisDigest
	}
	v, err := value.Unpack(rec.Value, true)
	if err != nil {
		return 0, GenesisDigest
	}
	hr := &headRecord{}
	if value.Unmarshal(v, hr) != nil || len(hr.Digest) != 32 {
		return 0, GenesisDigest
	}
	copy(digest[:], hr.Digest)
	return hr.Index, digest
}

// StoreHead persists the chain head. commitVersion is the raft index (envelope
// version) so every replica stamps the same value. Errors are non-fatal: the head
// recomputes from the log if a write is lost, so persistence is a snapshot
// optimization, not a correctness dependency for a running node.
func StoreHead(s headStore, index uint64, digest [32]byte, commitVersion uint64) {
	v, err := value.Marshal(&headRecord{Index: index, Digest: digest[:]})
	if err != nil {
		return
	}
	raw, err := value.Pack(v)
	if err != nil {
		return
	}
	_, _ = s.Put(&pb.RecordRequest{Key: headKey(), Value: raw}, commitVersion)
}
