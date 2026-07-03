/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"bytes"
	"encoding/binary"
	"go.arpabet.com/consensusdb/cdb"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/uuid"
	"golang.org/x/xerrors"
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


func DecodeKey(entryKey []byte) (*pb.Key, error) {

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
		var id uuid.UUID
		id.UnmarshalSortableBinary(b[j:])
		return &pb.Key{MajorKey: majorKey, RegionName: regionName, MinorKey: minorKey,
			Timestamp: &pb.TimeUUID{
				MostSigBits: id.MostSignificantBits(),
				LeastSigBits: id.LeastSignificantBits()}}, nil
	} else {
		return &pb.Key{MajorKey: majorKey, RegionName: regionName, MinorKey: minorKey}, nil
	}
}

func EncodeKey(key *pb.Key) (entryKey, rowKey []byte) {

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
		id := uuid.Create(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
		id.MarshalSortableBinaryTo(out[i:])
		return out, out[:i]
	}

	return out[:i], out[:i]

}

func EncodeKeyPrefix(key *pb.Key, lastField Field) ([]byte, error) {

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
		id := uuid.Create(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
		id.MarshalSortableBinaryTo(out[i:])
		i = i + 16

		if lastField == TimestampField {
			return out[:i], nil
		}

	}

	return out, xerrors.Errorf("key does not have the lastField %q", lastField)

}


func Equal(left *pb.Key, right *pb.Key) bool {

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



