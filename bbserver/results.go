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

package bbserver

import (
	"github.com/consensusdb/consensusdb/proto/bbproto"
	"github.com/dgraph-io/badger"
)

func HeadOf(timestamp uint64, item *badger.Item) *bbproto.Head {
	return &bbproto.Head{Version: item.Version(), ExpiresAt:item.ExpiresAt(), Timestamp: timestamp, DiskSize: item.EstimatedSize()}
}

func RecordOf(timestamp uint64, item *badger.Item, data []byte) *bbproto.Record {
	return &bbproto.Record{Head: HeadOf(timestamp, item), Value: data}
}

func SuccessGetNotFoundResult() *bbproto.TxOperationResult {

	get := new(bbproto.GetResult)

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.TxOperationResult_Get{get}

	return result
}

func SuccessHeadResult(timestamp uint64, item *badger.Item) *bbproto.TxOperationResult {

	get := new(bbproto.GetResult)
	get.Record = &bbproto.Record{Head: HeadOf(timestamp, item)}

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.TxOperationResult_Get{ Get: get }

	return result
}

func SuccessGetResult(timestamp uint64, item *badger.Item, data []byte) *bbproto.TxOperationResult {

	get := new(bbproto.GetResult)
	get.Record = RecordOf(timestamp, item, data)

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.TxOperationResult_Get{Get: get}

	return result
}

func SuccessRangeResult(records []*bbproto.Record) *bbproto.TxOperationResult {

	rang := new(bbproto.RangeResult)
	rang.Records = records

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.TxOperationResult_Range{Range: rang}

	return result
}

func SuccessTouchNotFoundResult() *bbproto.TxOperationResult {

	touch := new(bbproto.TouchResult)

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED

	result.Result = &bbproto.TxOperationResult_Touch{ touch }

	return result
}

func SuccessTouchResult(timestamp uint64, item *badger.Item, expiresAt uint64) *bbproto.TxOperationResult {

	touch := new(bbproto.TouchResult)
	touch.Head = HeadOf(timestamp, item)
	touch.Head.ExpiresAt = expiresAt

	result := new(bbproto.TxOperationResult)
	result.Status = bbproto.StatusCode_SUCCESS

	result.Result = &bbproto.TxOperationResult_Touch{ touch }

	return result
}

func SuccessPutResult(updated bool) *bbproto.TxOperationResult {

	result := new(bbproto.TxOperationResult)
	if updated {
		result.Status = bbproto.StatusCode_SUCCESS
	} else {
		result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &bbproto.TxOperationResult_Put{ &bbproto.PutResult{} }

	return result
}

func SuccessRemoveResult(updated bool) *bbproto.TxOperationResult {

	result := new(bbproto.TxOperationResult)
	if updated {
		result.Status = bbproto.StatusCode_SUCCESS
	} else {
		result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &bbproto.TxOperationResult_Remove{&bbproto.RemoveResult{}}

	return result
}
