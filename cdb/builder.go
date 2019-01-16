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
	"math"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/shvid/timeuuid"
	"math/rand"
	"fmt"
)


/**
	Key builder
 */

type KeyBuilder struct {
	key *cserverpb.Key
}

func NewKey() KeyBuilder {
	return KeyBuilder { key: &cserverpb.Key{} }
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
	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetUnixTimeMillis(timestampMillis)
	uuid.SetCounter(rand.Int63())
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

/**
	It will calculate SHA1 passwordHash of name (usually it is the context value) and override timestamp in Unix milliseconds to UUID

	Finally we will get unique TimeUUID based o content value and timestamp
 */

func (t KeyBuilder) WithNamedTimestamp(name []byte, timestampMillis int64) KeyBuilder {
	uuid, _ := timeuuid.NameUUIDFromBytes(name, timeuuid.NamebasedVer5)
	// it will override uuid to Time-based UUID
	uuid.SetUnixTimeMillis(timestampMillis)
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithTimestamp(uuid timeuuid.UUID) KeyBuilder {
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithMinTimestamp() KeyBuilder {

	uuidMin := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuidMin.SetTime100NanosUnsigned(0)
	uuidMin.SetMinCounter()

	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuidMin.MostSignificantBits(), LeastSigBits: uuidMin.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) WithMaxTimestamp() KeyBuilder {

	uuidMax := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuidMax.SetTime100NanosUnsigned(math.MaxUint64)
	uuidMax.SetMaxCounter()

	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuidMax.MostSignificantBits(), LeastSigBits: uuidMax.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) RemoveTimestamp() KeyBuilder {
	t.key.Timestamp = nil
	return t;
}

func (t KeyBuilder) Timestamp() timeuuid.UUID {
	return GetTimeUUID(t.key)
}

func GetTimeUUID(key *cserverpb.Key) timeuuid.UUID {
	if key.Timestamp != nil {
		return timeuuid.CreateUUID(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
	} else {
		return timeuuid.Empty
	}
}

func (t KeyBuilder) Build() Key {
	return t
}

func (t KeyBuilder) toProto() *cserverpb.Key {
	return t.key
}

/**
	Key request builder
 */

type KeyRequestBuilder struct {
	request *cserverpb.KeyRequest
}

func NewRequest(key Key) KeyRequestBuilder {
	return KeyRequestBuilder { request: &cserverpb.KeyRequest{ Key: key.toProto() } }
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

func (t KeyRequestBuilder) build() *cserverpb.KeyRequest {
	return t.request;
}

/**
	Range request builder
 */

type RangeRequestBuilder struct {
	request *cserverpb.RangeRequest
}

func NewRangeRequest(key Key) RangeRequestBuilder {
	return RangeRequestBuilder { request: &cserverpb.RangeRequest{ Key: key.toProto(), Type: cserverpb.RangeType_LESS_OR_EQUAL, NumRecords: 1 } }
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

func (t RangeRequestBuilder) build() *cserverpb.RangeRequest {
	return t.request;
}

/**
	Scan request builder
 */

type ScanRequestBuilder struct {
	request  *cserverpb.ScanRequest
}

func NewScanRequest() ScanRequestBuilder {
	return ScanRequestBuilder{ request: &cserverpb.ScanRequest{} }
}

func (t ScanRequestBuilder) HeadOnly() ScanRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t ScanRequestBuilder) build() *cserverpb.ScanRequest {
	return t.request;
}

/**
	Record request builder
 */

type RecordRequestBuilder struct {
	key         Key
	request     *cserverpb.RecordRequest
	compressor  Compressor
	cipher      Cipher
	cipherMode  CipherMode
	value       []byte
}

func NewRecord(key Key, value []byte) RecordRequestBuilder {
	return RecordRequestBuilder {
		key:        key,
		request: 	&cserverpb.RecordRequest{ Key: key.toProto(), Metadata: 0 },
		compressor: NO_COMPRESSION,
		cipher: 	NO_ENCRYPTION,
		cipherMode: NO_ENCRYPTION_MODE,
		value: 		value }
}

func NewRecordRequest(key Key) RecordRequestBuilder {
	return NewRecord(key, emptyValue)
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

func (t RecordRequestBuilder) build(keychain Keychain) (*cserverpb.RecordRequest, error) {

	value, err := PackValue(t.key, t.value, t.compressor, t.cipher, t.cipherMode, keychain)
	if err != nil {
		return t.request, err
	}

	t.request.Metadata |= t.compressor.MetadataFlag() | t.cipher.MetadataFlag() | t.cipherMode.MetadataFlag()
	t.request.Value = value

	return t.request, nil
}

