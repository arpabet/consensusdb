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

package cdb

import (
	"github.com/consensusdb/consensusdb/proto/bbproto"
	"github.com/pkg/errors"
	"fmt"
	"github.com/consensusdb/consensusdb/c"
)

type IOperation interface {

	WithMinorKey(minorKey []byte) IOperation

	HeadOnly() IOperation

	CompressOnServer() IOperation

	EncryptOnServer() IOperation

	WithTimestamp(timestamp uint64) IOperation

	OverrideTtl(ttlSeconds uint32) IOperation

	CompareAndSet(version uint64) IOperation

	toProto() *bbproto.TxOperation

}

type IHead interface {

	Version() uint64

	ExpiresAt() uint64

	Timestamp() uint64

	DiskSize() int64

}

type IRecord interface {

	Head() IHead

	Value() []byte

}

type IResult interface {

	GetStatus() int32

	Updated() bool

	Exists() bool

	GetRecord() IRecord

	GetRecords() []IRecord

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

func (this *EmptyHead) Version() uint64 {
	return 0
}

func (this *EmptyHead) ExpiresAt() uint64 {
	return 0
}

func (this *EmptyHead) Timestamp() uint64 {
	return 0
}

func (this *EmptyHead) DiskSize() int64 {
	return 0
}

type ProtoHead struct {
	head  *bbproto.Head
}

func (this *ProtoHead) Version() uint64 {
	return this.head.Version
}

func (this *ProtoHead) ExpiresAt() uint64 {
	return this.head.ExpiresAt
}

func (this *ProtoHead) Timestamp() uint64 {
	return this.head.Timestamp
}

func (this *ProtoHead) DiskSize() int64 {
	return this.head.DiskSize
}

var emptyRecord = EmptyRecord{}

var emptyRecords = []IRecord{&emptyRecord}

type EmptyRecord struct {
}

func (this *EmptyRecord) Head() IHead {
	return &emptyHead
}

func (this *EmptyRecord) Value() []byte {
	return emptyValue
}

type ProtoRecord struct {
	record  *bbproto.Record
}

func (this *ProtoRecord) Head() IHead {
	return &ProtoHead{this.record.Head}
}

func (this *ProtoRecord) Value() []byte {
	return this.record.Value
}

type HeadOnlyRecord struct {
	head   IHead
}

func (this *HeadOnlyRecord) Head() IHead {
	return this.head
}

func (this *HeadOnlyRecord) Value() []byte {
	return emptyValue
}

type GetOp struct {

	Key    bbproto.Key
	Get    bbproto.GetOperation

}

type RangeOp struct {

	Key      bbproto.Key
	Range    bbproto.RangeOperation

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

func Range(regionName string, majorKey []byte, numRecords int) IOperation {

	op := new(RangeOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey
	op.Range.NumRecords = uint32(numRecords)

	return op
}

func RangeReplicated(regionName string, numRecords int) IOperation {

	op := new(RangeOp)

	op.Key.RegionName = regionName
	op.Range.NumRecords = uint32(numRecords)

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
//  HeadOnly
//

func (this *GetOp) HeadOnly() IOperation {
	this.Get.HeadOnly = true
	return this
}

func (this *RangeOp) HeadOnly() IOperation {
	this.Range.HeadOnly = true
	return this
}

func (this *TouchOp) HeadOnly() IOperation {
	return this
}

func (this *PutOp) HeadOnly() IOperation {
	return this
}

func (this *RemoveOp) HeadOnly() IOperation {
	return this
}

//
//  CompressOnServer
//

func (this *GetOp) CompressOnServer() IOperation {
	return this
}

func (this *RangeOp) CompressOnServer() IOperation {
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

func (this *GetOp) EncryptOnServer() IOperation {
	return this
}

func (this *RangeOp) EncryptOnServer() IOperation {
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

func (this *GetOp) WithMinorKey(minorKey []byte) IOperation {
	this.Key.MinorKey = minorKey
	return this
}

func (this *RangeOp) WithMinorKey(minorKey []byte) IOperation {
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

func (this *GetOp) WithTimestamp(timestamp uint64) IOperation {
	this.Key.Timestamp = timestamp
	return this
}

func (this *RangeOp) WithTimestamp(timestamp uint64) IOperation {
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
//  OverrideTtl
//

func (this *GetOp) OverrideTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *RangeOp) OverrideTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *TouchOp) OverrideTtl(ttlSeconds uint32) IOperation {
	this.Touch.OverrideTtl = true
	this.Touch.TtlSeconds = ttlSeconds
	return this
}

func (this *PutOp) OverrideTtl(ttlSeconds uint32) IOperation {
	this.Put.OverrideTtl = true
	this.Put.TtlSeconds = ttlSeconds
	return this
}

func (this *RemoveOp) OverrideTtl(ttlSeconds uint32) IOperation {
	return this
}

//
//  CompareAndSet
//

func (this *GetOp) CompareAndSet(version uint64) IOperation {
	return this
}

func (this *RangeOp) CompareAndSet(version uint64) IOperation {
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

func (this* GetOp) toProto() *bbproto.TxOperation {

	op := new(bbproto.TxOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.TxOperation_Get{&this.Get}

	return op

}

func (this* RangeOp) toProto() *bbproto.TxOperation {

	op := new(bbproto.TxOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.TxOperation_Range{&this.Range}

	return op

}

func (this* TouchOp) toProto() *bbproto.TxOperation {

	op := new(bbproto.TxOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.TxOperation_Touch{&this.Touch}

	return op

}

func (this* PutOp) toProto() *bbproto.TxOperation {

	op := new(bbproto.TxOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.TxOperation_Put{&this.Put}

	return op

}

func (this* RemoveOp) toProto() *bbproto.TxOperation {

	op := new(bbproto.TxOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.TxOperation_Remove{ &this.Remove}

	return op

}


//
//
//  Results
//
//

type GetResult struct {
	Exist       bool
	Record      IRecord
}

type RangeResult struct {
	Exist         bool
	Records       []IRecord
}

type TouchResult struct {
	StatusCode      bbproto.StatusCode
	Exist           bool
	Head            IHead
}

type PutResult struct {
	StatusCode      bbproto.StatusCode
}

type RemoveResult struct {
	StatusCode      bbproto.StatusCode
}

type ErrorResult struct {
	StatusCode      bbproto.StatusCode
	Message         string
}

func ParseResult(result *bbproto.TxOperationResult) IResult {

	if c.IsSuccessResult(result)  {
		return ParseSuccessResult(result)
	} else {
		return &ErrorResult{result.Status, result.Message}
	}

}

func ParseGetResult(result *bbproto.GetResult) IResult {

	record := result.GetRecord()
	if record != nil {
		return &GetResult{Exist: true, Record: &ProtoRecord{record}}
	} else {
		return &GetResult{Exist: false, Record: &emptyRecord}
	}

}

func ParseRangeResult(result *bbproto.RangeResult) IResult {

	records := result.GetRecords()
	if records != nil {

		size := len(records)
		protoRecords := make([]IRecord, 0, size)

		for i := 0; i < size; i = i + 1 {
			protoRecords = append(protoRecords, &ProtoRecord{records[i]})
		}

		return &RangeResult{Exist: true, Records: protoRecords}
	} else {
		return &RangeResult{Exist: false, Records: emptyRecords}
	}

}

func ParseTouchResult(status bbproto.StatusCode, result *bbproto.TouchResult) IResult {

	head := result.GetHead()
	if head != nil {
		return &TouchResult{StatusCode: status, Exist: true, Head: &ProtoHead{head}}
	} else {
		return &TouchResult{StatusCode: status, Exist: false, Head: &emptyHead}
	}

}

func ParseSuccessResult(result *bbproto.TxOperationResult) IResult {

	switch result.Result.(type) {

	case *bbproto.TxOperationResult_Get:
		return ParseGetResult(result.GetGet())

	case *bbproto.TxOperationResult_Range:
		return ParseRangeResult(result.GetRange())

	case *bbproto.TxOperationResult_Touch:
		return ParseTouchResult(result.GetStatus(), result.GetTouch())

	case *bbproto.TxOperationResult_Put:
		return &PutResult{result.Status}

	case *bbproto.TxOperationResult_Remove:
		return &RemoveResult{result.Status}
	}

	return &ErrorResult{bbproto.StatusCode_ERROR_UNSUPPORTED, "client received wrong result type"}
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

func (this *GetResult) GetRecord() IRecord {
	return this.Record
}

func (this *GetResult) GetRecords() []IRecord {
	return []IRecord{this.Record}
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
// RangeResult implements IResult
//

func (this *RangeResult) GetStatus() int32 {
	return int32(bbproto.StatusCode_SUCCESS)
}

func (this *RangeResult) Updated() bool {
	return false
}

func (this *RangeResult) Exists() bool {
	return this.Exist
}

func (this *RangeResult) GetRecord() IRecord {
	return this.Records[0]
}

func (this *RangeResult) GetRecords() []IRecord {
	return this.Records
}

func (this *RangeResult) IsError() bool {
	return false
}

func (this *RangeResult) GetError() error {
	return &emptyError
}

func (this *RangeResult) GetMessage() string {
	return ""
}


//
// TouchResult implements IResult
//

func (this *TouchResult) GetStatus() int32 {
	return int32(this.StatusCode)
}

func (this *TouchResult) Updated() bool {
	return this.StatusCode == bbproto.StatusCode_SUCCESS
}

func (this *TouchResult) Exists() bool {
	return this.Exist
}

func (this *TouchResult) GetRecord() IRecord {
	return &HeadOnlyRecord{this.Head}
}

func (this *TouchResult) GetRecords() []IRecord {
	return []IRecord{this.GetRecord()}
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
	return int32(this.StatusCode)
}

func (this *PutResult) Updated() bool {
	return this.StatusCode == bbproto.StatusCode_SUCCESS
}

func (this *PutResult) Exists() bool {
	return true
}

func (this *PutResult) GetRecord() IRecord {
	return &emptyRecord
}

func (this *PutResult) GetRecords() []IRecord {
	return emptyRecords
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
	return int32(this.StatusCode)
}

func (this *RemoveResult) Updated() bool {
	return this.StatusCode == bbproto.StatusCode_SUCCESS
}

func (this *RemoveResult) Exists() bool {
	return false
}

func (this *RemoveResult) GetRecord() IRecord {
	return &emptyRecord
}

func (this *RemoveResult) GetRecords() []IRecord {
	return emptyRecords
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
	return int32(this.StatusCode)
}

func (this *ErrorResult) Updated() bool {
	return false
}

func (this *ErrorResult) Exists() bool {
	return false
}

func (this *ErrorResult) GetRecord() IRecord {
	return &emptyRecord
}

func (this *ErrorResult) GetRecords() []IRecord {
	return emptyRecords
}

func (this *ErrorResult) IsError() bool {
	return true
}

func (this *ErrorResult) GetError() error {
	return errors.New(this.StatusCode.String())
}

func (this *ErrorResult) GetMessage() string {
	return this.Message
}

func NewNetworkError(err error) IResult {
	return &ErrorResult{bbproto.StatusCode_ERROR_NETWORK, fmt.Sprint("remote error: ", err)}
}
