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
	minorKeyLen := len(key.MinorKey)

	sz := 2 + majorKeyLen + 2 + minorKeyLen

	if key.Timestamp > 0 {
		sz = sz + 8
	}

	return sz
}

func DecodeKey(b []byte) *cserverpb.Key {

	majorKeyLen := binary.BigEndian.Uint16(b)
	i := 2 + int(majorKeyLen)

	majorKey := b[2:i]

	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	j := i + 2
	i = j + int(minorKeyLen)

	minorKey := b[j:i]

	var timestamp uint64
	if i <= len(b)-8 {
		timestamp = binary.BigEndian.Uint64(b[i:])
	} else {
		timestamp = 0
	}

	return &cserverpb.Key{RegionName: "TEST", MajorKey: majorKey, MinorKey: minorKey, Timestamp: timestamp}
}

func GetKeyTimestamp(b []byte) uint64 {

	majorKeyLen := binary.BigEndian.Uint16(b)
	i := 2 + int(majorKeyLen)

	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(minorKeyLen)

	if i <= len(b)-8 {
		return binary.BigEndian.Uint64(b[i:])
	} else {
		return 0;
	}

}

func GetMajorKeyPrefix(majorKey []byte) []byte {

	majorKeyLen := binary.BigEndian.Uint16(majorKey)

	prefix := make([]byte, 2 + int(majorKeyLen))

	binary.BigEndian.PutUint16(prefix, uint16(majorKeyLen))
	copy(prefix[2:], majorKey)

	return prefix
}

func ReplaceKeyTimestamp(b []byte, timestamp uint64) []byte {

	majorKeyLen := binary.BigEndian.Uint16(b)
	i := 2 + int(majorKeyLen)

	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	i = i + 2 + int(minorKeyLen)

	other := make([]byte, i + 8)
	copy(other, b[:i])
	binary.BigEndian.PutUint64(other[i:], timestamp)

	return other
}

// return key length without timestamp
func EncodeKey(key *cserverpb.Key, out []byte) int {

	majorKeyLen := len(key.MajorKey)
	minorKeyLen := len(key.MinorKey)

	binary.BigEndian.PutUint16(out, uint16(majorKeyLen))

	copy(out[2:], key.MajorKey)
	i := 2 + majorKeyLen

	binary.BigEndian.PutUint16(out[i:], uint16(minorKeyLen))
	i = i + 2

	copy(out[i:], key.MinorKey)
	i = i + minorKeyLen

	if key.Timestamp > 0 {
		binary.BigEndian.PutUint64(out[i:], key.Timestamp)
	}

	return i
}

func EncodeKeyTo(key *cserverpb.Key, buf *bytes.Buffer) {

	var enc [8]byte
	enc16 := enc[:2]
	enc64 := enc[:8]

	majorKeyLen := len(key.MajorKey)
	minorKeyLen := len(key.MinorKey)

	binary.BigEndian.PutUint16(enc16, uint16(majorKeyLen))
	buf.Write(enc16)
	buf.Write(key.MajorKey)

	binary.BigEndian.PutUint16(enc16, uint16(minorKeyLen))
	buf.Write(enc16)
	buf.Write(key.MinorKey)

	if key.Timestamp > 0 {
		binary.BigEndian.PutUint64(enc64, key.Timestamp)
		buf.Write(enc64)
	}

}