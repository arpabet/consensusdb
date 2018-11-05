/*
 *
 * Copyright 2018-present Alexander Shvid and other authors
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
	"bigbagger/proto/bbproto"
	"github.com/dgraph-io/badger"
)

func HeadOf(timestamp uint64, item *badger.Item) *bbproto.Head {
	return &bbproto.Head{Version: item.Version(), ExpiresAt:item.ExpiresAt(), Timestamp: timestamp, DiskSize: item.EstimatedSize()}
}

func SuccessHeadNotFoundResult() *bbproto.RecordResult {

	head := new(bbproto.HeadResult)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Head{ head }

	return result
}

func SuccessHeadResult(timestamp uint64, item *badger.Item) *bbproto.RecordResult {

	head := new(bbproto.HeadResult)
	head.Head = HeadOf(timestamp, item)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Head{ head }

	return result
}

func SuccessGetNotFoundResult() *bbproto.RecordResult {

	get := new(bbproto.GetResult)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Get{get}

	return result
}

func SuccessGetResult(timestamp uint64, data []byte, item *badger.Item) *bbproto.RecordResult {

	get := new(bbproto.GetResult)
	get.Head = HeadOf(timestamp, item)
	get.Value = data

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Get{get}

	return result
}

func SuccessTouchNotFoundResult() *bbproto.RecordResult {

	touch := new(bbproto.TouchResult)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED

	result.Result = &bbproto.RecordResult_Touch{ touch }

	return result
}

func SuccessTouchResult(timestamp uint64, item *badger.Item, expiresAt uint64) *bbproto.RecordResult {

	touch := new(bbproto.TouchResult)
	touch.Head = HeadOf(timestamp, item)
	touch.Head.ExpiresAt = expiresAt

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS

	result.Result = &bbproto.RecordResult_Touch{ touch }

	return result
}

func SuccessPutResult(updated bool) *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	if updated {
		result.Status = bbproto.StatusCode_SUCCESS
	} else {
		result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &bbproto.RecordResult_Put{ &bbproto.PutResult{} }

	return result
}

func SuccessRemoveResult(updated bool) *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	if updated {
		result.Status = bbproto.StatusCode_SUCCESS
	} else {
		result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED
	}
	result.Result =  &bbproto.RecordResult_Remove{&bbproto.RemoveResult{}}

	return result
}
