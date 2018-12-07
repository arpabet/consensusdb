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
)

type Key struct {
	MajorKey    []byte    // compare first
	MinorKey    []byte    // compare second
	Timestamp   uint64    // compare third (optional)
}

func (k *Key) EncodedSize() int {

	majorKeyLen := len(k.MajorKey)
	minorKeyLen := len(k.MinorKey)

	sz := 2 + majorKeyLen + 2 + minorKeyLen

	if k.Timestamp > 0 {
		sz = sz + 8
	}

	return sz
}

func (k *Key) Decode(b []byte) {

	majorKeyLen := binary.BigEndian.Uint16(b)
	i := 2 + int(majorKeyLen)

	k.MajorKey = b[2:i]

	minorKeyLen := binary.BigEndian.Uint16(b[i:])
	j := i + 2
	i = j + int(minorKeyLen)

	k.MajorKey = b[j:i]

	if i <= len(b)-8 {
		k.Timestamp = binary.BigEndian.Uint64(b[i:])
	}

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
func (k *Key) Encode(b []byte) int {

	majorKeyLen := len(k.MajorKey)
	minorKeyLen := len(k.MinorKey)

	binary.BigEndian.PutUint16(b, uint16(majorKeyLen))

	copy(b[2:], k.MajorKey)
	i := 2 + majorKeyLen

	binary.BigEndian.PutUint16(b[i:], uint16(minorKeyLen))
	i = i + 2

	copy(b[i:], k.MinorKey)
	i = i + minorKeyLen

	if k.Timestamp > 0 {
		binary.BigEndian.PutUint64(b[i:], k.Timestamp)
	}

	return i
}

func (k *Key) EncodeTo(buf *bytes.Buffer) {

	var enc [8]byte
	enc16 := enc[:2]
	enc64 := enc[:8]

	majorKeyLen := len(k.MajorKey)
	minorKeyLen := len(k.MinorKey)

	binary.BigEndian.PutUint16(enc16, uint16(majorKeyLen))
	buf.Write(enc16)
	buf.Write(k.MajorKey)

	binary.BigEndian.PutUint16(enc16, uint16(minorKeyLen))
	buf.Write(enc16)
	buf.Write(k.MinorKey)

	if k.Timestamp > 0 {
		binary.BigEndian.PutUint64(enc64, k.Timestamp)
		buf.Write(enc64)
	}

}