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

type IRegionStore interface {

	GetName() string

	GetRegion() *cserverpb.Region

	NewTransaction() IRegionTnx

	Close() error

	GetSnapshot(majorKey []byte, outC chan<- *cserverpb.RawRecord) (error)

}

type IRegionTnx interface {

	Update(bool)

	Begin()

	ProcessOperation(operation *cserverpb.TxOperation) *cserverpb.TxOperationResult

	Rollback()

    Commit() error

}

type ErrorStore struct {
	regionName   string
	result       *cserverpb.TxOperationResult
}

type ErrorTxn struct {
	store        *ErrorStore
}

func (this *ErrorStore) GetName() string {
	return this.regionName
}

func (this *ErrorStore) GetRegion() *cserverpb.Region {
	return &cserverpb.Region{Name: this.regionName}
}

func (this *ErrorStore) NewTransaction() IRegionTnx {
	return &ErrorTxn{store: this}
}

func (this *ErrorStore) Close() error {
	return nil
}

func (this *ErrorStore)  GetSnapshot(majorKey []byte, outC chan<- *cserverpb.RawRecord) error {
	return nil
}

func (this *ErrorTxn) Update(update bool) {
}

func (this *ErrorTxn) Begin() {
}

func (this *ErrorTxn) ProcessOperation(operation *cserverpb.TxOperation) *cserverpb.TxOperationResult {
	return this.store.result
}

func (this *ErrorTxn) Rollback() {
}

func (this *ErrorTxn) Commit() error {
	return nil
}

func NewErrorStore(regionName string, result  *cserverpb.TxOperationResult) IRegionStore {
	return &ErrorStore{regionName, result}
}