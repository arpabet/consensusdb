package bbcommon

import "bigbagger/proto/bbproto"

func SuccessExistsResult(version uint64) *bbproto.RecordResult {

	exists := new(bbproto.ExistsResult)
	exists.Exists = version > 0
	exists.Timestamp = version

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Exists{ exists }

	return result
}

func SuccessGetResult(data []byte, version uint64) *bbproto.RecordResult {

	get := new(bbproto.GetResult)
	get.Value = data
	get.Timestamp = version
	get.Version = version

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Get{get}

	return result
}

func SuccessPutResult() *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Put{ &bbproto.PutResult{} }

	return result
}

func SuccessRemoveResult() *bbproto.RecordResult {

	result := new(bbproto.RecordResult)
	result.Status = bbproto.StatusCode_SUCCESS
	result.Result =  &bbproto.RecordResult_Remove{&bbproto.RemoveResult{}}

	return result
}
