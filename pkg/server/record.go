/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/pb"
	badger "github.com/dgraph-io/badger/v2"
)

func RecordNotFound(key *pb.Key) *pb.Record {
	return &pb.Record {  Key: key }
}

func RecordNotFetched(key *pb.Key, item *badger.Item) *pb.Record {
	head := &pb.Head{
		Version: item.Version(),
		ExpiresAt:item.ExpiresAt(),
		DiskSize: item.EstimatedSize(),
		Metadata: int32(item.UserMeta()),
	}
	return &pb.Record {  Key: key, Head: head }
}

func RecordFetched(key *pb.Key, item *badger.Item, data []byte) *pb.Record {
	head := &pb.Head{
		Version: item.Version(),
		ExpiresAt:item.ExpiresAt(),
		DiskSize: item.EstimatedSize(),
		Metadata: int32(item.UserMeta()),
	}
	return &pb.Record {  Key: key, Head: head, Value: data }
}
