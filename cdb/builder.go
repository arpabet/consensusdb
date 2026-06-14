/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cdb

import (
	"math"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/uuid"
	"math/rand"
	"fmt"
	"github.com/golang/protobuf/proto"
)


/**
	Key builder
 */

type KeyBuilder struct {
	key *pb.Key
}

func NewKey() KeyBuilder {
	return KeyBuilder { key: &pb.Key{} }
}

func (t KeyBuilder) WithMajorKey(majorKey string) KeyBuilder {
	t.key.MajorKey = []byte(majorKey)
	return t;
}

func (t KeyBuilder) SetMajorKey(majorKey []byte) KeyBuilder {
	t.key.MajorKey = majorKey
	return t;
}

func (t KeyBuilder) MajorKey() []byte {
	return t.key.MajorKey
}

func (t KeyBuilder) WithRegionName(regionName string) KeyBuilder {
	t.key.RegionName = []byte(regionName)
	return t;
}

func (t KeyBuilder) SetRegionName(regionName []byte) KeyBuilder {
	t.key.RegionName = regionName
	return t;
}

func (t KeyBuilder) RegionName() []byte {
	return t.key.RegionName
}

func (t KeyBuilder) WithMinorKey(minorKey string) KeyBuilder {
	t.key.MinorKey = []byte(minorKey)
	return t;
}

func (t KeyBuilder) SetMinorKey(minorKey []byte) KeyBuilder {
	t.key.MinorKey = minorKey
	return t;
}

func (t KeyBuilder) MinorKey() []byte {
	return t.key.MinorKey
}

func (t KeyBuilder) String() string {
	if t.key.Timestamp != nil {
		return fmt.Sprint(ToPrintable(t.MajorKey()), "/", ToPrintable(t.RegionName()), "/", ToPrintable(t.MinorKey()), "/", t.Timestamp())
	} else {
		return fmt.Sprint(ToPrintable(t.MajorKey()), "/", ToPrintable(t.RegionName()), "/", ToPrintable(t.MinorKey()))
	}
}


/**
	Generates random value for the second part of TimeUUID
 */

func (t KeyBuilder) WithRandTimestamp(timestampMillis int64) KeyBuilder {
	id := uuid.New(uuid.TimebasedVer1)
	id.SetUnixTimeMillis(timestampMillis)
	id.SetCounter(rand.Int63())
	t.key.Timestamp = &pb.TimeUUID{ MostSigBits: id.MostSignificantBits(), LeastSigBits: id.LeastSignificantBits() }
	return t;
}

/**
	It will calculate SHA1 passwordHash of name (usually it is the context value) and override timestamp in Unix milliseconds to UUID

	Finally we will get unique TimeUUID based o content value and timestamp
 */

func (t KeyBuilder) WithNamedTimestamp(name []byte, timestampMillis int64) KeyBuilder {
	id, _ := uuid.NameUUIDFromBytes(name, uuid.NamebasedVer5)
	// it will override id to Time-based UUID
	id.SetUnixTimeMillis(timestampMillis)
	t.key.Timestamp = &pb.TimeUUID{ MostSigBits: id.MostSignificantBits(), LeastSigBits: id.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithTimestamp(id uuid.UUID) KeyBuilder {
	t.key.Timestamp = &pb.TimeUUID{ MostSigBits: id.MostSignificantBits(), LeastSigBits: id.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithMinTimestamp() KeyBuilder {

	uuidMin := uuid.New(uuid.TimebasedVer1)
	uuidMin.SetTime100NanosUnsigned(0)
	uuidMin.SetMinCounter()

	t.key.Timestamp = &pb.TimeUUID{ MostSigBits: uuidMin.MostSignificantBits(), LeastSigBits: uuidMin.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithMaxTimestamp() KeyBuilder {

	uuidMax := uuid.New(uuid.TimebasedVer1)
	uuidMax.SetTime100NanosUnsigned(math.MaxUint64)
	uuidMax.SetMaxCounter()

	t.key.Timestamp = &pb.TimeUUID{ MostSigBits: uuidMax.MostSignificantBits(), LeastSigBits: uuidMax.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) RemoveTimestamp() KeyBuilder {
	t.key.Timestamp = nil
	return t;
}

func (t KeyBuilder) Timestamp() uuid.UUID {
	return GetTimeUUID(t.key)
}

func GetTimeUUID(key *pb.Key) uuid.UUID {
	if key.Timestamp != nil {
		return uuid.Create(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
	} else {
		return uuid.Empty
	}
}

func (t KeyBuilder) Build() Key {
	return t
}

func (t KeyBuilder) toProto() *pb.Key {
	return t.key
}

/**
	Key request builder
 */

type KeyRequestBuilder struct {
	request *pb.KeyRequest
}

func NewRequest(key Key) KeyRequestBuilder {
	return KeyRequestBuilder { request: &pb.KeyRequest{ Key: key.toProto() } }
}

func (t KeyRequestBuilder) HeadOnly() KeyRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t KeyRequestBuilder) WithTimeout(timeout int) KeyRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t KeyRequestBuilder) build() *pb.KeyRequest {
	return t.request;
}

/**
	Range request builder
 */

type RangeRequestBuilder struct {
	request *pb.RangeRequest
}

func NewRangeRequest(key Key) RangeRequestBuilder {
	return RangeRequestBuilder { request: &pb.RangeRequest{ Key: key.toProto(), Type: pb.RangeType_LESS_OR_EQUAL, NumRecords: 1 } }
}

func (t RangeRequestBuilder) WithNumRecords(numRecords int) RangeRequestBuilder {
	if numRecords > math.MaxInt32 {
		numRecords = math.MaxInt32
	}
	t.request.NumRecords = int32(numRecords)
	return t;
}

func (t RangeRequestBuilder) HeadOnly() RangeRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t RangeRequestBuilder) WithTimeout(timeout int) RangeRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t RangeRequestBuilder) build() *pb.RangeRequest {
	return t.request;
}

/**
	Scan request builder
 */

type ScanRequestBuilder struct {
	request  *pb.ScanRequest
}

func NewScanRequest() ScanRequestBuilder {
	return ScanRequestBuilder{ request: &pb.ScanRequest{} }
}

func (t ScanRequestBuilder) HeadOnly() ScanRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t ScanRequestBuilder) build() *pb.ScanRequest {
	return t.request;
}

/**
	Record request builder
 */

type RecordRequestBuilder struct {
	key         Key
	request     *pb.RecordRequest
	compressor  Compressor
	cipher      Cipher
	cipherMode  CipherMode
	value       []byte
	pb          proto.Message
}

func NewRecord(key Key, value []byte) RecordRequestBuilder {
	return RecordRequestBuilder {
		key:        key,
		request: 	&pb.RecordRequest{ Key: key.toProto(), Metadata: 0 },
		compressor: NO_COMPRESSION,
		cipher: 	NO_ENCRYPTION,
		cipherMode: NO_ENCRYPTION_MODE,
		value: 		value }
}

func NewRecordRequest(key Key) RecordRequestBuilder {
	return NewRecord(key, emptyValue)
}

func NewProtoRecord(key Key, pb proto.Message) RecordRequestBuilder {
	b := NewRecord(key, emptyValue)
	b.pb = pb
	return b
}

func (t RecordRequestBuilder) SetMetadata(metadata int32) RecordRequestBuilder {
	t.request.Metadata = metadata
	return t;
}

func (t RecordRequestBuilder) WithTtlSeconds(ttlSeconds int) RecordRequestBuilder {
	t.request.TtlSeconds = int64(ttlSeconds)
	return t;
}

func (t RecordRequestBuilder) SetTtlSeconds(ttlSeconds int) RecordRequestBuilder {
	t.request.TtlSeconds = int64(ttlSeconds)
	return t;
}

func (t RecordRequestBuilder) OnlyIfAbsent() RecordRequestBuilder {
	t.request.CompareAndSet = true
	t.request.Version 		= 0
	return t;
}

func (t RecordRequestBuilder) CompareAndSet(version uint64) RecordRequestBuilder {
	t.request.CompareAndSet = true
	t.request.Version 		= version
	return t;
}

func (t RecordRequestBuilder) SetValue(value []byte) RecordRequestBuilder {
	t.value = value
	t.pb = nil
	return t;
}

func (t RecordRequestBuilder) WithValue(pb proto.Message) RecordRequestBuilder {
	t.value = emptyValue
	t.pb = pb
	return t;
}

func (t RecordRequestBuilder) UseCompression(compressor Compressor) RecordRequestBuilder {
	t.compressor = compressor
	return t;
}

func (t RecordRequestBuilder) UseEncryption(cipher Cipher, cipherMode CipherMode) RecordRequestBuilder {
	t.cipher = cipher
	t.cipherMode = cipherMode
	return t;
}

func (t RecordRequestBuilder) WithTimeout(timeout int) RecordRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t RecordRequestBuilder) build(keychain Keychain) (*pb.RecordRequest, error) {

	// override t.value
	if t.pb != nil && len(t.value) == 0 {
		var err error
		t.value, err = proto.Marshal(t.pb)
		if err != nil {
			return t.request, err
		}
	}

	value, err := PackValue(t.key, t.value, t.compressor, t.cipher, t.cipherMode, keychain)
	if err != nil {
		return t.request, err
	}

	t.request.Metadata |= t.compressor.MetadataFlag() | t.cipher.MetadataFlag() | t.cipherMode.MetadataFlag()
	t.request.Value = value

	return t.request, nil
}

