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

package c

import (
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
)

func ErrorRegionNotFound(name string) *cserverpb.TxOperationResult {

	res := new(cserverpb.TxOperationResult)
	res.Status = cserverpb.StatusCode_ERROR_NO_REGION
	res.Message = name

	return res
}

func ErrorBadRequest(message string) *cserverpb.TxOperationResult {

	res := new(cserverpb.TxOperationResult)
	res.Status = cserverpb.StatusCode_ERROR_BAD_REQUEST
	res.Message = message

	return res
}

func ErrorUnsupported(message string) *cserverpb.TxOperationResult {

	res := new(cserverpb.TxOperationResult)
	res.Status = cserverpb.StatusCode_ERROR_UNSUPPORTED
	res.Message = message

	return res
}

func ErrorDriver(message string) *cserverpb.TxOperationResult {

	res := new(cserverpb.TxOperationResult)
	res.Status = cserverpb.StatusCode_ERROR_DRIVER
	res.Message = message

	return res
}

func IsSuccessResult(result *cserverpb.TxOperationResult) bool {
	return result.Status == cserverpb.StatusCode_SUCCESS || result.Status == cserverpb.StatusCode_SUCCESS_NOT_UPDATED
}