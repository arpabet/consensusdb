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

func HeadOf(timestamp uint64, item *badger.Item) *cserverpb.Head {
	return &cserverpb.Head{Version: item.Version(), ExpiresAt:item.ExpiresAt(), Timestamp: timestamp, DiskSize: item.EstimatedSize()}
}

func RecordOf(timestamp uint64, item *badger.Item, data []byte) *cserverpb.Record {
	return &cserverpb.Record{Head: HeadOf(timestamp, item), Value: data}
}

func SuccessGetNotFoundResult() *cserverpb.TxOperationResult {

	get := new(cserverpb.GetResult)

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS
	result.Result =  &cserverpb.TxOperationResult_Get{get}

	return result
}

func SuccessHeadResult(timestamp uint64, item *badger.Item) *cserverpb.TxOperationResult {

	get := new(cserverpb.GetResult)
	get.Record = &cserverpb.Record{Head: HeadOf(timestamp, item)}

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS
	result.Result =  &cserverpb.TxOperationResult_Get{ Get: get }

	return result
}

func SuccessGetResult(timestamp uint64, item *badger.Item, data []byte) *cserverpb.TxOperationResult {

	get := new(cserverpb.GetResult)
	get.Record = RecordOf(timestamp, item, data)

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS
	result.Result =  &cserverpb.TxOperationResult_Get{Get: get}

	return result
}

func SuccessRangeResult(records []*cserverpb.Record) *cserverpb.TxOperationResult {

	rang := new(cserverpb.RangeResult)
	rang.Records = records

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS
	result.Result =  &cserverpb.TxOperationResult_Range{Range: rang}

	return result
}

func SuccessTouchNotFoundResult() *cserverpb.TxOperationResult {

	touch := new(cserverpb.TouchResult)

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS_NOT_UPDATED

	result.Result = &cserverpb.TxOperationResult_Touch{ touch }

	return result
}

func SuccessTouchResult(timestamp uint64, item *badger.Item, expiresAt uint64) *cserverpb.TxOperationResult {

	touch := new(cserverpb.TouchResult)
	touch.Head = HeadOf(timestamp, item)
	touch.Head.ExpiresAt = expiresAt

	result := new(cserverpb.TxOperationResult)
	result.Status = cserverpb.StatusCode_SUCCESS

	result.Result = &cserverpb.TxOperationResult_Touch{ touch }

	return result
}

func SuccessPutResult(updated bool) *cserverpb.TxOperationResult {

	result := new(cserverpb.TxOperationResult)
	if updated {
		result.Status = cserverpb.StatusCode_SUCCESS
	} else {
		result.Status = cserverpb.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &cserverpb.TxOperationResult_Put{ &cserverpb.PutResult{} }

	return result
}

func SuccessRemoveResult(updated bool) *cserverpb.TxOperationResult {

	result := new(cserverpb.TxOperationResult)
	if updated {
		result.Status = cserverpb.StatusCode_SUCCESS
	} else {
		result.Status = cserverpb.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &cserverpb.TxOperationResult_Remove{&cserverpb.RemoveResult{}}

	return result
}
