/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/pb"
)

func RecordNotFound(key *pb.Key) *pb.Record {
	return &pb.Record {  Key: key }
}

// RecordHead builds a header-only record. version and expiresAt come from the
// value envelope (see FetchRecord), not from badger's node-local item metadata.
func RecordHead(key *pb.Key, version, expiresAt uint64, diskSize int64, metadata int32) *pb.Record {
	head := &pb.Head{
		Version:   version,
		ExpiresAt: expiresAt,
		DiskSize:  diskSize,
		Metadata:  metadata,
	}
	return &pb.Record {  Key: key, Head: head }
}

// RecordValue builds a full record (header + unwrapped value).
func RecordValue(key *pb.Key, version, expiresAt uint64, diskSize int64, metadata int32, value []byte) *pb.Record {
	record := RecordHead(key, version, expiresAt, diskSize, metadata)
	record.Value = value
	return record
}
