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

package bbclient

import (
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"github.com/pkg/errors"
	"fmt"
)

type IOperation interface {

	WithMinorKey(minorKey []byte) IOperation

	CompressOnServer() IOperation

	EncryptOnServer() IOperation

	WithTimestamp(timestamp uint64) IOperation

	WithTtl(ttlSeconds uint32) IOperation

	CompareAndSet(version uint64) IOperation

	toProto() *bbproto.RecordOperation

}

type IHead interface {

	GetVersion() uint64     // committedAt

	GetExpiresAt() uint64

	GetTimestamp() uint64

	GetDiskSize() int64

}

type IResult interface {

	GetStatus() int32

	Updated() bool

	Exists() bool

	GetHead() IHead

	GetValue() []byte

	IsError() bool

	GetError() error

	GetMessage() string

}

type EmptyError struct {
}

func (this* EmptyError) Error() string {
	return ""
}

var emptyError = EmptyError{}

var emptyValue = []byte{}

var emptyHead = EmptyHead{}

type EmptyHead struct {
}

func (this *EmptyHead) GetVersion() uint64 {
	return 0
}

func (this *EmptyHead) GetExpiresAt() uint64 {
	return 0
}

func (this *EmptyHead) GetTimestamp() uint64 {
	return 0
}

func (this *EmptyHead) GetDiskSize() int64 {
	return 0
}

type ProtoHead struct {
	head  *bbproto.Head
}

func (this *ProtoHead) GetVersion() uint64 {
	return this.head.Version
}

func (this *ProtoHead) GetExpiresAt() uint64 {
	return this.head.ExpiresAt
}

func (this *ProtoHead) GetTimestamp() uint64 {
	return this.head.Timestamp
}

func (this *ProtoHead) GetDiskSize() int64 {
	return this.head.DiskSize
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

func Head(regionName string, majorKey []byte) IOperation {

	op := new(HeadOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	return op
}

func HeadReplicated(regionName string) IOperation {

	op := new(HeadOp)

	op.Key.RegionName = regionName

	return op
}

func Get(regionName string, majorKey []byte) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	return op
}

func GetReplicated(regionName string) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName

	return op
}

func Touch(regionName string, majorKey []byte) IOperation {

	op := new(TouchOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	return op
}

func TouchReplicated(regionName string) IOperation {

	op := new(TouchOp)

	op.Key.RegionName = regionName

	return op
}

func Put(regionName string, majorKey, value []byte) IOperation {

	op := new(PutOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	op.Put.Value = value

	return op
}

func PutReplicated(regionName string, value []byte) IOperation {

	op := new(PutOp)

	op.Key.RegionName = regionName
	op.Put.Value = value

	return op
}

func Remove(regionName string, majorKey []byte) IOperation {

	op := new(RemoveOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	return op
}

func RemoveReplicated(regionName string) IOperation {

	op := new(RemoveOp)

	op.Key.RegionName = regionName

	return op
}

//
//  CompressOnServer
//

func (this *HeadOp) CompressOnServer() IOperation {
	return this
}

func (this *GetOp) CompressOnServer() IOperation {
	return this
}

func (this *TouchOp) CompressOnServer() IOperation {
	return this
}

func (this *PutOp) CompressOnServer() IOperation {
	this.Put.CompressOnServer = true
	return this
}

func (this *RemoveOp) CompressOnServer() IOperation {
	return this
}


//
//  EncryptOnServer
//

func (this *HeadOp) EncryptOnServer() IOperation {
	return this
}

func (this *GetOp) EncryptOnServer() IOperation {
	return this
}

func (this *TouchOp) EncryptOnServer() IOperation {
	return this
}

func (this *PutOp) EncryptOnServer() IOperation {
	this.Put.EncryptOnServer = true
	return this
}

func (this *RemoveOp) EncryptOnServer() IOperation {
	return this
}

//
//  WithMinorKey
//

func (this *HeadOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
	return this
}

func (this *GetOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
	return this
}

func (this *TouchOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
	return this
}

func (this *PutOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
	return this
}

func (this *RemoveOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
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
	Exist       bool
	Head        IHead
}

type GetResult struct {
	Exist       bool
	Head        IHead
	Value       []byte
}

type TouchResult struct {
	Status      bbproto.StatusCode
	Exist       bool
	Head        IHead
}

type PutResult struct {
	Status      bbproto.StatusCode
}

type RemoveResult struct {
	Status      bbproto.StatusCode
}

type ErrorResult struct {
	Status      bbproto.StatusCode
	Message     string
}

func ParseResult(result *bbproto.RecordResult) IResult {

	if result.Status == bbproto.StatusCode_SUCCESS || result.Status == bbproto.StatusCode_SUCCESS_NOT_UPDATED  {
		return ParseSuccessResult(result)
	} else {
		return &ErrorResult{result.Status, result.Message}
	}

}

func ParseHeadResult(result *bbproto.HeadResult) IResult {

	head := result.GetHead()
	if head != nil {
		return &HeadResult{Exist: true, Head: &ProtoHead{head}}
	} else {
		return &HeadResult{Exist: false, Head: &emptyHead}
	}

}

func ParseGetResult(result *bbproto.GetResult) IResult {

	head := result.GetHead()
	if head != nil {
		return &GetResult{Exist: true, Head: &ProtoHead{head}, Value: result.GetValue()}
	} else {
		return &GetResult{Exist: false, Head: &emptyHead}
	}

}

func ParseTouchResult(status bbproto.StatusCode, result *bbproto.TouchResult) IResult {

	head := result.GetHead()
	if head != nil {
		return &TouchResult{Status: status, Exist: true, Head: &ProtoHead{head}}
	} else {
		return &TouchResult{Status: status, Exist: false, Head: &emptyHead}
	}

}

func ParseSuccessResult(result *bbproto.RecordResult) IResult {

	switch result.Result.(type) {

	case *bbproto.RecordResult_Head:
		return ParseHeadResult(result.GetHead())

	case *bbproto.RecordResult_Get:
		return ParseGetResult(result.GetGet())

	case *bbproto.RecordResult_Touch:
		return ParseTouchResult(result.GetStatus(), result.GetTouch())

	case *bbproto.RecordResult_Put:
		return &PutResult{result.Status}

	case *bbproto.RecordResult_Remove:
		return &RemoveResult{result.Status}
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
	return this.Exist
}

func (this *HeadResult) GetHead() IHead {
	return this.Head
}

func (this *HeadResult) GetValue() []byte {
	return emptyValue
}

func (this *HeadResult) IsError() bool {
	return false
}

func (this *HeadResult) GetError() error {
	return &emptyError
}

func (this *HeadResult) GetMessage() string {
	return ""
}

//
// GetResult implements IResult
//

func (this *GetResult) GetStatus() int32 {
	return int32(bbproto.StatusCode_SUCCESS)
}

func (this *GetResult) Updated() bool {
	return false
}

func (this *GetResult) Exists() bool {
	return this.Exist
}

func (this *GetResult) GetHead() IHead {
	return this.Head
}

func (this *GetResult) GetValue() []byte {
	return this.Value
}

func (this *GetResult) IsError() bool {
	return false
}

func (this *GetResult) GetError() error {
	return &emptyError
}

func (this *GetResult) GetMessage() string {
	return ""
}


//
// TouchResult implements IResult
//

func (this *TouchResult) GetStatus() int32 {
	return int32(this.Status)
}

func (this *TouchResult) Updated() bool {
	return this.Status == bbproto.StatusCode_SUCCESS
}

func (this *TouchResult) Exists() bool {
	return this.Exist
}

func (this *TouchResult) GetHead() IHead {
	return this.Head
}

func (this *TouchResult) GetValue() []byte {
	return emptyValue
}

func (this *TouchResult) IsError() bool {
	return false
}

func (this *TouchResult) GetError() error {
	return &emptyError
}

func (this *TouchResult) GetMessage() string {
	return ""
}

//
// PutResult implements IResult
//

func (this *PutResult) GetStatus() int32 {
	return int32(this.Status)
}

func (this *PutResult) Updated() bool {
	return this.Status == bbproto.StatusCode_SUCCESS
}

func (this *PutResult) Exists() bool {
	return true
}

func (this *PutResult) GetHead() IHead {
	return &emptyHead
}

func (this *PutResult) GetValue() []byte {
	return emptyValue
}

func (this *PutResult) IsError() bool {
	return false
}

func (this *PutResult) GetError() error {
	return &emptyError
}

func (this *PutResult) GetMessage() string {
	return ""
}

//
// RemoveResult implements IResult
//

func (this *RemoveResult) GetStatus() int32 {
	return int32(this.Status)
}

func (this *RemoveResult) Updated() bool {
	return this.Status == bbproto.StatusCode_SUCCESS
}

func (this *RemoveResult) Exists() bool {
	return false
}

func (this *RemoveResult) GetHead() IHead {
	return &emptyHead
}

func (this *RemoveResult) GetValue() []byte {
	return emptyValue
}

func (this *RemoveResult) IsError() bool {
	return false
}

func (this *RemoveResult) GetError() error {
	return &emptyError
}

func (this *RemoveResult) GetMessage() string {
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

func (this *ErrorResult) GetHead() IHead {
	return &emptyHead
}

func (this *ErrorResult) GetValue() []byte {
	return emptyValue
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

func NewNetworkError(err error) IResult {
	return &ErrorResult{bbproto.StatusCode_ERROR_NETWORK, fmt.Sprint("remote error: ", err)}
}
