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

package bbclient

import (
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
)

type IOperation interface {

	WithPartitionKey(key []byte) IOperation

	WithTimestamp(timestamp uint64) IOperation

	WithTtl(ttlSeconds uint32) IOperation

	CompareAndSet(version uint64) IOperation

	toProto() *bbproto.RecordOperation

}

type IResult interface {

	GetStatus() int32

	Updated() bool

	Exists() bool

	GetValue() []byte

	GetVersion() uint64     // committedAt

	GetExpiresAt() uint64

	GetTimestamp() uint64

	IsError() bool

	GetError() error

	GetMessage() string

}


type HeadOp struct {

	Key     bbproto.Key
	Head    bbproto.HeadOperation

}


type GetOp struct {

	Key    bbproto.Key
	Get    bbproto.GetOperation

}

type TouchOp struct {

	Key     bbproto.Key
	Touch   bbproto.TouchOperation

}

type PutOp struct {

	Key     bbproto.Key
	Put     bbproto.PutOperation

}

type RemoveOp struct {

	Key     bbproto.Key
	Remove  bbproto.RemoveOperation

}

func Head(setName string, key []byte) IOperation {

	op := new(HeadOp)

	op.Key.SetName = setName
	op.Key.RecordKey = key

	return op
}

func Get(setName string, key []byte) IOperation {

	op := new(GetOp)

	op.Key.SetName = setName
	op.Key.RecordKey = key

	return op
}

func Touch(setName string, key []byte) IOperation {

	op := new(TouchOp)

	op.Key.SetName = setName
	op.Key.RecordKey = key

	return op
}

func Put(setName string, key, value []byte) IOperation {

	op := new(PutOp)

	op.Key.SetName = setName
	op.Key.RecordKey = key

	op.Put.Value = value

	return op
}

func Remove(setName string, key []byte) IOperation {

	op := new(RemoveOp)

	op.Key.SetName = setName
	op.Key.RecordKey = key

	return op
}

//
//  WithPartitionKey
//

func (this *HeadOp) WithPartitionKey(key []byte) IOperation {
	this.Key.PartitionKey = key
	return this
}

func (this *GetOp) WithPartitionKey(key []byte) IOperation {
	this.Key.PartitionKey = key
	return this
}

func (this *TouchOp) WithPartitionKey(key []byte) IOperation {
	this.Key.PartitionKey = key
	return this
}

func (this *PutOp) WithPartitionKey(key []byte) IOperation {
	this.Key.PartitionKey = key
	return this
}

func (this *RemoveOp) WithPartitionKey(key []byte) IOperation {
	this.Key.PartitionKey = key
	return this
}

//
//  WithTimestamp
//

func (this *HeadOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

func (this *GetOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

func (this *TouchOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

func (this *PutOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

func (this *RemoveOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

//
//  WithTtl
//

func (this *HeadOp) WithTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *GetOp) WithTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *TouchOp) WithTtl(ttlSeconds uint32) IOperation {
	this.Touch.OverrideTtl = true
	this.Touch.TtlSeconds = ttlSeconds
	return this
}

func (this *PutOp) WithTtl(ttlSeconds uint32) IOperation {
	this.Put.OverrideTtl = true
	this.Put.TtlSeconds = ttlSeconds
	return this
}

func (this *RemoveOp) WithTtl(ttlSeconds uint32) IOperation {
	return this
}

//
//  CompareAndSet
//

func (this *HeadOp) CompareAndSet(version uint64) IOperation {
	return this
}

func (this *GetOp) CompareAndSet(version uint64) IOperation {
	return this
}

func (this *TouchOp) CompareAndSet(version uint64) IOperation {
	return this
}

func (this *PutOp) CompareAndSet(version uint64) IOperation {
	this.Put.CompareAndSet = true
	this.Put.Version = version
	return this
}

func (this *RemoveOp) CompareAndSet(version uint64) IOperation {
	return this
}

//
//  ToProto
//


func (this* HeadOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Head{&this.Head}

	return op

}

func (this* GetOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Get{&this.Get}

	return op

}

func (this* TouchOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Touch{&this.Touch}

	return op

}

func (this* PutOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Put{&this.Put}

	return op

}

func (this* RemoveOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Remove{ &this.Remove}

	return op

}


//
//
//  Results
//
//


type HeadResult struct {
	Version     uint64    // committedAt, exists if > 0
	ExpiresAt   uint64
	Timestamp   uint64    // key part of timestamp for PIT
}

type UpdatedResult struct {
	Status      bbproto.StatusCode
	Result      bool
}

type ValueResult struct {
	Value       []byte
	Version     uint64    // committedAt, exists if > 0
	ExpiresAt   uint64
	Timestamp   uint64    // key part of timestamp for PIT
}

type ErrorResult struct {
	Status      bbproto.StatusCode
	Message     string
}

func ParseResult(result *bbproto.RecordResult) IResult {

	if result.Status == bbproto.StatusCode_SUCCESS {
		return ParseSuccessResult(result)
	} else if result.Status == bbproto.StatusCode_SUCCESS_NOT_UPDATED {
		return &UpdatedResult{result.Status, false}
	} else {
		return &ErrorResult{result.Status, result.Message}
	}

}

func ParseSuccessResult(result *bbproto.RecordResult) IResult {

	switch result.Result.(type) {

	case *bbproto.RecordResult_Head:
		{
			head := result.GetHead()
			return &HeadResult{head.Version, head.ExpiresAt, head.Timestamp}
		}

	case *bbproto.RecordResult_Get:
		{
			get := result.GetGet()
			return &ValueResult{get.Value, get.Version, get.ExpiresAt, get.Timestamp}
		}

	case *bbproto.RecordResult_Touch:
		return &UpdatedResult{result.Status, true}

	case *bbproto.RecordResult_Put:
		return &UpdatedResult{result.Status, true}

	case *bbproto.RecordResult_Remove:
		return &UpdatedResult{result.Status, true }
	}

	return &ErrorResult{bbproto.StatusCode_ERROR_UNSUPPORTED, "client received wrong result type"}
}

//
// HeadResult implements IResult
//

func (this *HeadResult) GetStatus() int32 {
	return int32(bbproto.StatusCode_SUCCESS)
}

func (this *HeadResult) Updated() bool {
	return false
}

func (this *HeadResult) Exists() bool {
	return this.Version > 0
}

func (this *HeadResult) GetVersion() uint64 {
	return this.Version
}

func (this *HeadResult) GetValue() []byte {
	return nil
}

func (this *HeadResult) GetExpiresAt() uint64 {
	return this.ExpiresAt
}

func (this *HeadResult) GetTimestamp() uint64 {
	return this.Timestamp
}

func (this *HeadResult) IsError() bool {
	return false
}

func (this *HeadResult) GetError() error {
	return nil
}

func (this *HeadResult) GetMessage() string {
	return ""
}

//
// UpdatedResult implements IResult
//

func (this *UpdatedResult) GetStatus() int32 {
	return int32(this.Status)
}

func (this *UpdatedResult) Updated() bool {
	return this.Result
}

func (this *UpdatedResult) Exists() bool {
	return true
}

func (this *UpdatedResult) GetVersion() uint64 {
	return 0
}

func (this *UpdatedResult) GetValue() []byte {
	return nil
}

func (this *UpdatedResult) GetExpiresAt() uint64 {
	return 0
}

func (this *UpdatedResult) GetTimestamp() uint64 {
	return 0
}

func (this *UpdatedResult) IsError() bool {
	return false
}

func (this *UpdatedResult) GetError() error {
	return nil
}

func (this *UpdatedResult) GetMessage() string {
	return ""
}


//
// ValueResult implements IResult
//

func (this *ValueResult) GetStatus() int32 {
	return int32(bbproto.StatusCode_SUCCESS)
}

func (this *ValueResult) Updated() bool {
	return false
}

func (this *ValueResult) Exists() bool {
	return this.Version > 0
}

func (this *ValueResult) GetVersion() uint64 {
	return this.Version
}

func (this *ValueResult) GetValue() []byte {
	return this.Value
}

func (this *ValueResult) GetExpiresAt() uint64 {
	return this.ExpiresAt
}

func (this *ValueResult) GetTimestamp() uint64 {
	return this.Timestamp
}

func (this *ValueResult) IsError() bool {
	return false
}

func (this *ValueResult) GetError() error {
	return nil
}

func (this *ValueResult) GetMessage() string {
	return ""
}

//
// ErrorResult implements IResult
//

func (this *ErrorResult) GetStatus() int32 {
	return int32(this.Status)
}

func (this *ErrorResult) Updated() bool {
	return false
}

func (this *ErrorResult) Exists() bool {
	return false
}

func (this *ErrorResult) GetVersion() uint64 {
	return 0
}

func (this *ErrorResult) GetValue() []byte {
	return nil
}

func (this *ErrorResult) GetExpiresAt() uint64 {
	return 0
}

func (this *ErrorResult) GetTimestamp() uint64 {
	return 0
}

func (this *ErrorResult) IsError() bool {
	return true
}

func (this *ErrorResult) GetError() error {
	return errors.New(this.Status.String())
}

func (this *ErrorResult) GetMessage() string {
	return this.Message
}
