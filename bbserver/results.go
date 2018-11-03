package bbserver

import (
	"bigbagger/proto/bbproto"
	"github.com/dgraph-io/badger"
)

func SuccessHeadNotFoundResult() *bbproto.RecordResult {

	head := new(bbproto.HeadResult)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Head{ head }

	return result
}

func SuccessHeadResult(timestamp uint64, item *badger.Item) *bbproto.RecordResult {

	head := new(bbproto.HeadResult)
	head.Version = item.Version()
	head.ExpiresAt = item.ExpiresAt()
	head.Timestamp = timestamp

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
	get.Value = data
	get.Version = item.Version()
	get.ExpiresAt = item.ExpiresAt()
	get.Timestamp = timestamp

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Get{get}

	return result
}

func SuccessTouchResult() *bbproto.RecordResult {

	touch := new(bbproto.TouchResult)

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result = &bbproto.RecordResult_Touch{ touch }

	return result
}

func SuccessPutResult() *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Put{ &bbproto.PutResult{} }

	return result
}

func SuccessPutNotUpdatedResult() *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS_NOT_UPDATED
	result.Result =  &bbproto.RecordResult_Put{ &bbproto.PutResult{} }

	return result
}

func SuccessRemoveResult() *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Remove{&bbproto.RemoveResult{}}

	return result
}
