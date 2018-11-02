package bbclient

import (
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
)

type IOperation interface {

	toProto() *bbproto.RecordOperation

}

type IResult interface {

	GetStatus() int32

	Updated() bool

	Exists() bool

	GetVersion() int64

	GetValue() []byte

	GetTimestamp() uint64

	IsError() bool

	GetError() error

	GetErrorMessage() string

}

type IKey interface {

	SetDataset(dataset string) IKey

	SetPatKey(patKey []byte) IKey

	SetRecordKey(recordKey []byte) IKey

	SetTimestamp(timestamp uint64) IKey

}

type Key struct {

	Dataset              string
	PatKey               []byte
	RecordKey            []byte
	Timestamp            uint64

}

func (this* Key) SetDataset(dataset string) IKey {
	this.Dataset = dataset
	return this
}

func (this* Key) SetPatKey(patKey []byte) IKey {
	this.PatKey = patKey
	return this
}

func (this* Key) SetRecordKey(recordKey []byte) IKey {
	this.RecordKey = recordKey
	return this
}

func (this* Key) SetTimestamp(timestamp uint64) IKey {
	this.Timestamp = timestamp
	return this
}


type Get struct {

	Key Key

}

func (this* Get) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)

	return op

}

type Exists struct {

	Key Key

}

func (this* Exists) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)

	return op

}

type Touch struct {

	Key Key

}

func (this* Touch) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)

	return op

}

type Put struct {

	Key Key

	CompareAndSet bool
	Version       int64
	Value         []byte
	TtlSeconds    int32

}

func (this* Put) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)

	return op

}

type Remove struct {

	Key Key

}

func (this* Remove) toProto() *bbproto.RecordOperation {

	op := new(bbproto.RecordOperation)

	return op

}

type ExistsResult struct {
	Result     bool
	Timestamp  uint64
}

type UpdatedResult struct {
	Status     bbproto.StatusCode
	Result     bool
}

type ValueResult struct {
	Version    int64
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
		return &ErrorResult{result.Status, result.ErrorMessage}
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

func (this *ExistsResult) GetVersion() int64 {
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

func (this *UpdatedResult) GetVersion() int64 {
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

func (this *ValueResult) GetVersion() int64 {
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

func (this *ErrorResult) GetVersion() int64 {
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
