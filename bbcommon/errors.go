package bbcommon

import "bigbagger/proto/bbproto"

func ErrorDatasetNotFound(name string) *bbproto.RecordResult {

	res := new(bbproto.RecordResult)
	res.Status = bbproto.StatusCode_ERROR_NO_DATASET
	res.Message = "dataset not found: " + name

	return res
}

func ErrorBadRequest(message string) *bbproto.RecordResult {

	res := new(bbproto.RecordResult)
	res.Status = bbproto.StatusCode_ERROR_BAD_REQUEST
	res.Message = message

	return res
}

func ErrorUnsupported(message string) *bbproto.RecordResult {

	res := new(bbproto.RecordResult)
	res.Status = bbproto.StatusCode_ERROR_UNSUPPORTED
	res.Message = message

	return res
}

func ErrorDriver(message string) *bbproto.RecordResult {

	res := new(bbproto.RecordResult)
	res.Status = bbproto.StatusCode_ERROR_DRIVER
	res.Message = message

	return res
}