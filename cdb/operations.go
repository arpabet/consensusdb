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
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/pkg/errors"
	"github.com/consensusdb/consensusdb/c"
)

type IOperation interface {

	WithMinorKey(minorKey []byte) IOperation

	HeadOnly() IOperation

	CompressOnServer() IOperation

	EncryptOnServer() IOperation

	WithTimestamp(timestamp uint64) IOperation

	WithTtl(ttlSeconds uint32) IOperation

	CompareAndSet(version uint64) IOperation

	toProto() *cserverpb.TxOperation

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
	head  *cserverpb.Head
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

var emptyRecords = []IRecord{}

type EmptyRecord struct {
}

func (this *EmptyRecord) Head() IHead {
	return &emptyHead
}

func (this *EmptyRecord) Value() []byte {
	return emptyValue
}

type ProtoRecord struct {
	record  *cserverpb.Record
}

func (this *ProtoRecord) Head() IHead {
	return &ProtoHead{this.record.Head}
}

func (this *ProtoRecord) Value() []byte {
	return this.record.Value
}

type GetOp struct {

	Key cserverpb.Key
	Get cserverpb.GetOperation

}

type RangeOp struct {

	Key   cserverpb.Key
	Range cserverpb.RangeOperation

}

type TouchOp struct {

	Key   cserverpb.Key
	Touch cserverpb.TouchOperation

}

type PutOp struct {

	Key cserverpb.Key
	Put cserverpb.PutOperation

}

type RemoveOp struct {

	Key    cserverpb.Key
	Remove cserverpb.RemoveOperation

}

func Range(regionName string, majorKey, startMinorKey, endMinorKey []byte) IOperation {

	op := new(RangeOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey
	op.Key.MinorKey = startMinorKey
	op.Range.EndMinorKey = endMinorKey

	return op
}

func RangeReplicated(regionName string, startMinorKey, endMinorKey []byte) IOperation {

	op := new(RangeOp)

	op.Key.RegionName = regionName
	op.Key.MinorKey = startMinorKey
	op.Range.EndMinorKey = endMinorKey

	return op
}

func Get(regionName string, majorKey []byte) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey

	return op
}

func GetEarly(regionName string, majorKey []byte, lessOrEqualRecords int) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName
	op.Key.MajorKey = majorKey
	op.Get.LessOrEqualRecords = uint32(lessOrEqualRecords)

	return op
}

func GetReplicated(regionName string) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName

	return op
}

func GetEarlyReplicated(regionName string, lessOrEqualRecords int) IOperation {

	op := new(GetOp)

	op.Key.RegionName = regionName
	op.Get.LessOrEqualRecords = uint32(lessOrEqualRecords)

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
//  WithTtl
//

func (this *GetOp) WithTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *RangeOp) WithTtl(ttlSeconds uint32) IOperation {
	return this
}

func (this *TouchOp) WithTtl(ttlSeconds uint32) IOperation {
	this.Touch.TtlSeconds = ttlSeconds
	return this
}

func (this *PutOp) WithTtl(ttlSeconds uint32) IOperation {
	this.Put.TtlSeconds = ttlSeconds
	return this
}

func (this *RemoveOp) WithTtl(ttlSeconds uint32) IOperation {
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

func (this* GetOp) toProto() *cserverpb.TxOperation {

	op := new(cserverpb.TxOperation)
	op.Key = &this.Key
	op.Operation = &cserverpb.TxOperation_Get{&this.Get}

	return op

}

func (this* RangeOp) toProto() *cserverpb.TxOperation {

	op := new(cserverpb.TxOperation)
	op.Key = &this.Key
	op.Operation = &cserverpb.TxOperation_Range{&this.Range}

	return op

}

func (this* TouchOp) toProto() *cserverpb.TxOperation {

	op := new(cserverpb.TxOperation)
	op.Key = &this.Key
	op.Operation = &cserverpb.TxOperation_Touch{&this.Touch}

	return op

}

func (this* PutOp) toProto() *cserverpb.TxOperation {

	op := new(cserverpb.TxOperation)
	op.Key = &this.Key
	op.Operation = &cserverpb.TxOperation_Put{&this.Put}

	return op

}

func (this* RemoveOp) toProto() *cserverpb.TxOperation {

	op := new(cserverpb.TxOperation)
	op.Key = &this.Key
	op.Operation = &cserverpb.TxOperation_Remove{ &this.Remove}

	return op

}


//
//
//  Results
//
//


type OperationResult struct {
	StatusCode    cserverpb.StatusCode
	Message       string
	Records       []IRecord
}

//
// OperationResult implements IResult
//

func (this *OperationResult) GetStatus() int32 {
	return int32(this.StatusCode)
}

func (this *OperationResult) Updated() bool {
	return this.StatusCode == cserverpb.StatusCode_SUCCESS
}

func (this *OperationResult) Exists() bool {
	return len(this.Records) > 0
}

func (this *OperationResult) GetRecord() IRecord {
	if len(this.Records) > 0 {
		return this.Records[0]
	} else {
		return &emptyRecord
	}
}

func (this *OperationResult) GetRecords() []IRecord {
	return this.Records
}

func (this *OperationResult) IsError() bool {
	return !c.IsSuccessCode(this.StatusCode)
}

func (this *OperationResult) GetError() error {
	if c.IsSuccessCode(this.StatusCode) {
		return nil
	} else {
		return errors.New(this.StatusCode.String())
	}
}

func (this *OperationResult) GetMessage() string {
	return this.Message
}

func ParseResult(result *cserverpb.TxOperationResult) IResult {

	records := result.Records
	size := len(records)

	if size > 0 {

		protoRecords := make([]IRecord, 0, size)

		for i := 0; i < size; i = i + 1 {
			protoRecords = append(protoRecords, &ProtoRecord{records[i]})
		}

		return &OperationResult{StatusCode: result.Status, Message: result.Message, Records: protoRecords}

	} else {
		return &OperationResult{StatusCode: result.Status, Message: result.Message, Records: emptyRecords}
	}

}

func NewErrorResult(status cserverpb.StatusCode, message string) IResult {
	return &OperationResult{StatusCode: status, Message: message, Records: emptyRecords}
}