package bbclient

import (
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
)

type IOperation interface {

	WithPartitionKey(key []byte) IOperation

	WithTimestamp(timestamp uint64) IOperation

	WithTtl(ttlSeconds int32) IOperation

	CompareAndSet(version uint64) IOperation

	toProto() *bbproto.RecordOperation

}

type IResult interface {

	GetStatus() int32

	Updated() bool

	Exists() bool

	GetVersion() uint64

	GetValue() []byte

	GetTimestamp() uint64

	IsError() bool

	GetError() error

	GetErrorMessage() string

}

type IKey interface {

	SetSetName(setName string) IKey

	SetPartitionKey(patKey []byte) IKey

	SetRecordKey(recordKey []byte) IKey

	SetRecordKeyString(recordKey string) IKey

	SetTimestamp(timestamp uint64) IKey

}

type ExistsOp struct {

	Key    bbproto.Key

}


type GetOp struct {

	Key    bbproto.Key

}

type TouchOp struct {

	Key    bbproto.Key
	Touch  bbproto.TouchOperation

}

type PutOp struct {

	Key    bbproto.Key
	Put    bbproto.PutOperation

}

type RemoveOp struct {

	Key    bbproto.Key

}

func Exists(setName string, key []byte) IOperation {

	op := new(ExistsOp)

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

func (this *ExistsOp) WithPartitionKey(key []byte) IOperation {
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

func (this *ExistsOp) WithTimestamp(timestamp uint64) IOperation {
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

func (this *ExistsOp) WithTtl(ttlSeconds int32) IOperation {
	return this
}

func (this *GetOp) WithTtl(ttlSeconds int32) IOperation {
	return this
}

func (this *TouchOp) WithTtl(ttlSeconds int32) IOperation {
	this.Touch.TtlSeconds = ttlSeconds
	return this
}

func (this *PutOp) WithTtl(ttlSeconds int32) IOperation {
	this.Put.TtlSeconds = ttlSeconds
	return this
}

func (this *RemoveOp) WithTtl(ttlSeconds int32) IOperation {
	return this
}

//
//  CompareAndSet
//

func (this *ExistsOp) CompareAndSet(version uint64) IOperation {
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


func (this* ExistsOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Exists{&bbproto.ExistsOperation{}}

	return op

}

func (this* GetOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Get{&bbproto.GetOperation{}}

	return op

}

func (this* TouchOp) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)
	op.Key = &this.Key
	op.Operation = &bbproto.RecordOperation_Touch{&bbproto.TouchOperation{}}

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
	op.Operation = &bbproto.RecordOperation_Remove{ &bbproto.RemoveOperation{} }

	return op

}


//
//
//  Results
//
//


type ExistsResult struct {
	Result     bool
	Timestamp  uint64
}

type UpdatedResult struct {
	Status     bbproto.StatusCode
	Result     bool
}

type ValueResult struct {
	Version    uint64
	Value      []byte
	Timestamp  uint64
}

type ErrorResult struct {
	Status       bbproto.StatusCode
	ErrorMessage string
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

	case *bbproto.RecordResult_Exists:
		{
			exists := result.GetExists()
			return &ExistsResult{exists.Exists, exists.Timestamp}
		}

	case *bbproto.RecordResult_Get:
		{
			get := result.GetGet()
			return &ValueResult{get.Version, get.Value, get.Timestamp}
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
// ExistsResult implements IResult
//

func (this *ExistsResult) GetStatus() int32 {
	return int32(bbproto.StatusCode_SUCCESS)
}

func (this *ExistsResult) Updated() bool {
	return false
}

func (this *ExistsResult) Exists() bool {
	return this.Result
}

func (this *ExistsResult) GetVersion() uint64 {
	return 0
}

func (this *ExistsResult) GetValue() []byte {
	return nil
}

func (this *ExistsResult) GetTimestamp() uint64 {
	return this.Timestamp
}

func (this *ExistsResult) IsError() bool {
	return false
}

func (this *ExistsResult) GetError() error {
	return nil
}

func (this *ExistsResult) GetErrorMessage() string {
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

func (this *UpdatedResult) GetTimestamp() uint64 {
	return 0
}

func (this *UpdatedResult) IsError() bool {
	return false
}

func (this *UpdatedResult) GetError() error {
	return nil
}

func (this *UpdatedResult) GetErrorMessage() string {
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
	return this.Value != nil
}

func (this *ValueResult) GetVersion() uint64 {
	return this.Version
}

func (this *ValueResult) GetValue() []byte {
	return this.Value
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

func (this *ValueResult) GetErrorMessage() string {
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

func (this *ErrorResult) GetTimestamp() uint64 {
	return 0
}

func (this *ErrorResult) IsError() bool {
	return true
}

func (this *ErrorResult) GetError() error {
	return errors.New(this.Status.String() + ": " + this.GetErrorMessage())
}

func (this *ErrorResult) GetErrorMessage() string {
	return this.ErrorMessage
}
