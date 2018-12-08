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
)

const (
	REGION_JSON = "region.json"
)

type IStorage interface {

	NewTransaction() IStorageTnx

	Close() error

	GetSnapshot(majorKey []byte, outC chan<- *cserverpb.RawRecord) (error)

}

type IStorageTnx interface {

	SetUpdate(bool)

	Begin()

	ProcessOperation(operation *cserverpb.TxOperation) *cserverpb.TxOperationResult

	Rollback()

    Commit() error

}

type ErrorStorage struct {
	result       *cserverpb.TxOperationResult
}

type ErrorStorageTxn struct {
	store        *ErrorStorage
}

func (this *ErrorStorage) NewTransaction() IStorageTnx {
	return &ErrorStorageTxn{store: this}
}

func (this *ErrorStorage) Close() error {
	return nil
}

func (this *ErrorStorage)  GetSnapshot(majorKey []byte, outC chan<- *cserverpb.RawRecord) error {
	return nil
}

func (this *ErrorStorageTxn) SetUpdate(update bool) {
}

func (this *ErrorStorageTxn) Begin() {
}

func (this *ErrorStorageTxn) ProcessOperation(operation *cserverpb.TxOperation) *cserverpb.TxOperationResult {
	return this.store.result
}

func (this *ErrorStorageTxn) Rollback() {
}

func (this *ErrorStorageTxn) Commit() error {
	return nil
}

func NewErrorStorage(result  *cserverpb.TxOperationResult) IStorage {
	return &ErrorStorage{result}
}