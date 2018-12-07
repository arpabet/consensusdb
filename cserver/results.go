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


var emptyRecords = []*cserverpb.Record{}
var emptyValue = []byte{}

func HeadOf(timestamp uint64, item *badger.Item) *cserverpb.Head {
	return &cserverpb.Head{Version: item.Version(), ExpiresAt:item.ExpiresAt(), Timestamp: timestamp, DiskSize: item.EstimatedSize()}
}

func RecordHeadOf(timestamp uint64, item *badger.Item) *cserverpb.Record {
	return RecordOf(timestamp, item, emptyValue)
}

func RecordOf(timestamp uint64, item *badger.Item, data []byte) *cserverpb.Record {
	return &cserverpb.Record{Head: HeadOf(timestamp, item), Value: data}
}

func SuccessResultOf(records []*cserverpb.Record) *cserverpb.TxOperationResult  {
	return &cserverpb.TxOperationResult{Status:cserverpb.StatusCode_SUCCESS, Records: records}
}

func SuccessResult() *cserverpb.TxOperationResult  {
	return &cserverpb.TxOperationResult{Status:cserverpb.StatusCode_SUCCESS, Records: emptyRecords}
}

func SuccessNotUpdatedResult() *cserverpb.TxOperationResult  {
	return &cserverpb.TxOperationResult{Status:cserverpb.StatusCode_SUCCESS_NOT_UPDATED, Records: emptyRecords}
}


