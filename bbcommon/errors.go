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

package bbcommon

import (
	"bigbagger/proto/bbproto"
)

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