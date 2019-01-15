/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
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

package cserver

import (
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/dgraph-io/badger"
)

func RecordNotFound(key *cserverpb.Key) *cserverpb.Record {
	return &cserverpb.Record {  Key: key }
}

func RecordNotFetched(key *cserverpb.Key, item *badger.Item) *cserverpb.Record {
	head := &cserverpb.Head{
		Version: item.Version(),
		ExpiresAt:item.ExpiresAt(),
		DiskSize: item.EstimatedSize(),
		Metadata: int32(item.UserMeta()),
	}
	return &cserverpb.Record {  Key: key, Head: head }
}

func RecordFetched(key *cserverpb.Key, item *badger.Item, data []byte) *cserverpb.Record {
	head := &cserverpb.Head{
		Version: item.Version(),
		ExpiresAt:item.ExpiresAt(),
		DiskSize: item.EstimatedSize(),
		Metadata: int32(item.UserMeta()),
	}
	return &cserverpb.Record {  Key: key, Head: head, Value: data }
}
