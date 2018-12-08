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
)


func GetEncodedSize(key *cserverpb.Key) int {

	majorKeyLen := len(key.MajorKey)
	regionNameLen := len(key.RegionName)
	minorKeyLen := len(key.MinorKey)

	size := 2 + majorKeyLen + 2 + regionNameLen + 2 + minorKeyLen

	if key.Timestamp > 0 {
		size = size + 8
	}

	return size
}

func DecodeKey(b []byte) *cserverpb.Key {

	j := 0

	//
	// MajorKey
	//
	majorKeyLen := binary.BigEndian.Uint16(b[j:])
	i := j + 2
	j = i + int(majorKeyLen)
	majorKey := b[i:j]

	//
	// RegionName
	//
	regionNameLen := binary.BigEndian.Uint16(b[j:])
	i = j + 2
	j = i + int(regionNameLen)
	regionName := b[i:j]

	//
	// MinorKey
	//
	minorKeyLen := binary.BigEndian.Uint16(b[j:])
	i = j + 2
	j = i + int(minorKeyLen)
	minorKey := b[i:j]

	//
	// Get Timestamp
	//
	var timestamp uint64
	if j <= len(b)-8 {
		timestamp = binary.BigEndian.Uint64(b[j:])
	} else {
		timestamp = 0
	}

	return &cserverpb.Key{RegionName: string(regionName), MajorKey: majorKey, MinorKey: minorKey, Timestamp: timestamp}
}

func GetKeyTimestamp(b []byte) uint64 {

	i := 0

	//
	// MajorKey
	//
	majorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(majorKeyLen)

	//
	// RegionName
	//
	regionNameLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(regionNameLen)

	//
	// MinorKey
	//
	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(minorKeyLen)

	//
	// Get Timestamp
	//
	if i <= len(b)-8 {
		return binary.BigEndian.Uint64(b[i:])
	} else {
		return 0;
	}

}

func GetMajorKeyPrefix(majorKey []byte) []byte {

	majorKeyLen := len(majorKey)

	p := make([]byte, 2 + majorKeyLen)

	//
	// MajorKey
	//
	binary.BigEndian.PutUint16(p, uint16(majorKeyLen))
	copy(p[2:], majorKey)

	return p
}

func GetRegionNamePrefix(majorKey []byte, regionName string) []byte {

	majorKeyLen := len(majorKey)
	regionNameLen := len(regionName)

	p := make([]byte, 2 + majorKeyLen + 2 + regionNameLen)

	i := 0

	//
	// MajorKey
	//
	binary.BigEndian.PutUint16(p[i:], uint16(majorKeyLen))
	i = i + 2
	copy(p[i:], majorKey)
	i = i + majorKeyLen

	//
	// RegionName
	//
	binary.BigEndian.PutUint16(p[i:], uint16(regionNameLen))
	i = i + 2
	copy(p[i:], regionName)

	return p
}

func ReplaceKeyTimestamp(b []byte, timestamp uint64) []byte {

	i := 0

	//
	// MajorKey
	//
	majorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(majorKeyLen)

	//
	// RegionName
	//
	regionNameLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(regionNameLen)

	//
	// MinorKey
	//
	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(minorKeyLen)

	//
	// Replace Timestamp
	//
	other := make([]byte, i + 8)
	copy(other, b[:i])
	binary.BigEndian.PutUint64(other[i:], timestamp)

	return other
}

// return key length without timestamp
func EncodeKey(key *cserverpb.Key, out []byte) int {

	majorKeyLen := len(key.MajorKey)
	regionNameLen := len(key.RegionName)
	minorKeyLen := len(key.MinorKey)

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
	// Timestamp
	//
	if key.Timestamp > 0 {
		binary.BigEndian.PutUint64(out[i:], key.Timestamp)
	}

	return i
}

func EncodeKeyTo(key *cserverpb.Key, buf *bytes.Buffer) {

	//
	// tmp buf
	//
	var enc [8]byte
	enc16 := enc[:2]
	enc64 := enc[:8]

	majorKeyLen := len(key.MajorKey)
	regionNameLen := len(key.RegionName)
	minorKeyLen := len(key.MinorKey)

	//
	// MajorKey
	//
	binary.BigEndian.PutUint16(enc16, uint16(majorKeyLen))
	buf.Write(enc16)
	buf.Write(key.MajorKey)

	//
	// RegionName
	//
	binary.BigEndian.PutUint16(enc16, uint16(regionNameLen))
	buf.Write(enc16)
	buf.Write([]byte(key.RegionName))

	//
	// MinorKey
	//
	binary.BigEndian.PutUint16(enc16, uint16(minorKeyLen))
	buf.Write(enc16)
	buf.Write(key.MinorKey)

	//
	// Timestamp
	//
	if key.Timestamp > 0 {
		binary.BigEndian.PutUint64(enc64, key.Timestamp)
		buf.Write(enc64)
	}

}


func IsEquals(left *cserverpb.Key, right *cserverpb.Key) bool {

	if !bytes.Equal(left.MajorKey, right.MajorKey) {
		return false
	}

	if left.RegionName != right.RegionName {
		return false
	}

	if !bytes.Equal(left.MinorKey, right.MinorKey) {
		return false
	}

	if left.Timestamp != right.Timestamp {
		return false
	}

	return true
}