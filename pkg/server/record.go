/*
 *
 * Copyright 2020-present Arpabet Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package server

import (
	"github.com/consensusdb/consensusdb/pkg/pb"
	"github.com/dgraph-io/badger"
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
