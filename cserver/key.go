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
	"encoding/binary"
	"bytes"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/shvid/timeuuid"
	"github.com/pkg/errors"
	"github.com/consensusdb/consensusdb/cdb"
)


const (
	MaxKeyLength = 65535
	SizeOfUUID = 16
)

type Field int

const (
	MajorKeyField   = Field(iota)
	RegionNameField
	MinorKeyField
	TimestampField
)

func SanitizeKeyLen(len int) int {
	if len > MaxKeyLength {
		len = MaxKeyLength
	}
	return len
}


func DecodeKey(entryKey []byte) (*cserverpb.Key, error) {

	b := cdb.CopyOf(entryKey)

	len := len(b)
	j := 0

	if len < 6 {
		return nil, ErrorIndexOutOfBounds
	}

	//
	// MajorKey
	//
	majorKeyLen := binary.BigEndian.Uint16(b[j:])
	i := j + 2
	j = i + int(majorKeyLen)

	if j > len {
		return nil, ErrorIndexOutOfBounds
	}

	majorKey := b[i:j]

	//
	// RegionName
	//
	regionNameLen := binary.BigEndian.Uint16(b[j:])
	i = j + 2
	j = i + int(regionNameLen)

	if j > len {
		return nil, ErrorIndexOutOfBounds
	}

	regionName := b[i:j]

	//
	// MinorKey
	//
	minorKeyLen := binary.BigEndian.Uint16(b[j:])
	i = j + 2
	j = i + int(minorKeyLen)

	if j > len {
		return nil, ErrorIndexOutOfBounds
	}

	minorKey := b[i:j]

	//
	// Get TimeUUID
	//
	if j <= len - SizeOfUUID {
		var uuid timeuuid.UUID
		uuid.UnmarshalSortableBinary(b[j:])
		return &cserverpb.Key{MajorKey: majorKey, RegionName: regionName, MinorKey: minorKey,
			Timestamp: &cserverpb.TimeUUID{
				MostSigBits: uuid.MostSignificantBits(),
				LeastSigBits: uuid.LeastSignificantBits()}}, nil
	} else {
		return &cserverpb.Key{MajorKey: majorKey, RegionName: regionName, MinorKey: minorKey}, nil
	}
}

func EncodeKey(key *cserverpb.Key) (entryKey, rowKey []byte) {

	majorKeyLen := SanitizeKeyLen(len(key.MajorKey))
	regionNameLen := SanitizeKeyLen(len(key.RegionName))
	minorKeyLen := SanitizeKeyLen(len(key.MinorKey))

	out := make([]byte, 2 + majorKeyLen + 2 + regionNameLen + 2 + minorKeyLen + SizeOfUUID)

	i := 0

	//
	// MajorKey
	//
	binary.BigEndian.PutUint16(out[i:], uint16(majorKeyLen))
	i = i + 2
	copy(out[i:], key.MajorKey)
	i = i + majorKeyLen

	//
	// RegionName
	//
	binary.BigEndian.PutUint16(out[i:], uint16(regionNameLen))
	i = i + 2
	copy(out[i:], key.RegionName)
	i = i + regionNameLen

	//
	// MinorKey
	//
	binary.BigEndian.PutUint16(out[i:], uint16(minorKeyLen))
	i = i + 2
	copy(out[i:], key.MinorKey)
	i = i + minorKeyLen

	//
	// TimeUUID
	//
	if key.Timestamp != nil {
		uuid := timeuuid.CreateUUID(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
		uuid.MarshalSortableBinaryTo(out[i:])
		return out, out[:i]
	}

	return out[:i], out[:i]

}

func EncodeKeyPrefix(key *cserverpb.Key, lastField Field) ([]byte, error) {

	majorKeyLen := SanitizeKeyLen(len(key.MajorKey))
	regionNameLen := SanitizeKeyLen(len(key.RegionName))
	minorKeyLen := SanitizeKeyLen(len(key.MinorKey))

	out := make([]byte, 2 + majorKeyLen + 2 + regionNameLen + 2 + minorKeyLen + SizeOfUUID)

	i := 0

	//
	// MajorKey
	//
	binary.BigEndian.PutUint16(out[i:], uint16(majorKeyLen))
	i = i + 2
	copy(out[i:], key.MajorKey)
	i = i + majorKeyLen

	if lastField == MajorKeyField {
		return out[:i], nil
	}

	//
	// RegionName
	//
	binary.BigEndian.PutUint16(out[i:], uint16(regionNameLen))
	i = i + 2
	copy(out[i:], key.RegionName)
	i = i + regionNameLen

	if lastField == RegionNameField {
		return out[:i], nil
	}

	//
	// MinorKey
	//
	binary.BigEndian.PutUint16(out[i:], uint16(minorKeyLen))
	i = i + 2
	copy(out[i:], key.MinorKey)
	i = i + minorKeyLen

	if lastField == MinorKeyField {
		return out[:i], nil
	}

	//
	// TimeUUID
	//
	if key.Timestamp != nil {
		uuid := timeuuid.CreateUUID(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
		uuid.MarshalSortableBinaryTo(out[i:])
		i = i + 16

		if lastField == TimestampField {
			return out[:i], nil
		}

	}

	return out, errors.Errorf("key does not have the lastField %q", lastField)

}


func Equal(left *cserverpb.Key, right *cserverpb.Key) bool {

	if !bytes.Equal(left.MajorKey, right.MajorKey) {
		return false
	}

	if !bytes.Equal(left.RegionName, right.RegionName) {
		return false
	}

	if !bytes.Equal(left.MinorKey, right.MinorKey) {
		return false
	}

	if left.Timestamp != nil {
		if right.Timestamp == nil {
			return false
		}
		if left.Timestamp.MostSigBits != right.Timestamp.MostSigBits {
			return false
		}
		if left.Timestamp.LeastSigBits != right.Timestamp.LeastSigBits {
			return false
		}
		return true
	}

	// if both left and right timestamps are nil
	return right.Timestamp == nil
}



