/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"fmt"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/uuid"
	"math/rand"
	"testing"
	"time"
)

func TestKey(t *testing.T) {

	majorKey := []byte("alex")
	regionName := []byte("accounts")
	minorKey := []byte("Bank1")

	rawKeyLen := 2 + len(majorKey) + 2 + len(regionName) + 2 + len(minorKey)

	//
	// No TimeUUID
	//



	key := &pb.Key{MajorKey: majorKey, RegionName: regionName, MinorKey: minorKey}

	entryKey, rawKey := EncodeKey(key)

	if len(entryKey) != rawKeyLen {
		t.Fatal("wrong encoded entry key len")
	}

	if len(rawKey) != rawKeyLen {
		t.Fatal("wrong encoded raw key len")
	}

	fmt.Print("No timestamp: ", entryKey, "\n")

	other, err := DecodeKey(entryKey)
	if err != nil {
		t.Fatal("can not decode ", err)
	}

	if !Equal(key, other) {
		t.Fatal("decoded wrong key")
	}

	other, err = DecodeKey(rawKey)
	if err != nil {
		t.Fatal("can not decode ", err)
	}

	if !Equal(key, other) {
		t.Fatal("decoded wrong key")
	}

	//
	// With Timestamp
	//

	id := uuid.New(uuid.TimebasedVer1)
	id.SetTime(time.Now())
	id.SetCounter(rand.Int63())

	fmt.Print("timbased uuid=", id.String(), "\n")

	key.Timestamp = &pb.TimeUUID{ MostSigBits:id.MostSignificantBits(), LeastSigBits: id.LeastSignificantBits() }

	entryKey, rawKey = EncodeKey(key)

	if len(entryKey) != rawKeyLen + 16 {
		t.Fatal("wrong entry key len with timestamp")
	}

	if len(rawKey) != rawKeyLen {
		t.Fatal("wrong raw key len with timestamp")
	}

	fmt.Print("With timestamp: ", entryKey, "\n")

	other, err = DecodeKey(entryKey)
	if err != nil {
		t.Fatal("can not decode ", err)
	}

	if !Equal(key, other) {
		t.Fatal("wrong decode with timestamp")
	}

	// Test Prefix

	majorKeyPrefix, err := EncodeKeyPrefix(key, MajorKeyField)

	if err != nil {
		t.Fatal("can not get prefix ", err)
	}

	if len(majorKeyPrefix) != 2 + len(majorKey) {
		t.Fatal("wrong major key prefix")
	}

	regionNamePrefix, err := EncodeKeyPrefix(key, RegionNameField)

	if err != nil {
		t.Fatal("can not get prefix ", err)
	}

	if len(regionNamePrefix) != 2 + len(majorKey) + 2 + len(regionName) {
		t.Fatal("wrong major key prefix")
	}

	minorKeyPrefix, err := EncodeKeyPrefix(key, MinorKeyField)

	if err != nil {
		t.Fatal("can not get prefix ", err)
	}

	if len(minorKeyPrefix) != rawKeyLen {
		t.Fatal("wrong major key prefix")
	}

	entryKey, err = EncodeKeyPrefix(key, TimestampField)

	if err != nil {
		t.Fatal("can not get prefix ", err)
	}

	if len(entryKey) != rawKeyLen + 16 {
		t.Fatal("wrong major key prefix")
	}

}
