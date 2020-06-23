/*
 *
 * Copyright 2020-present Arpabet Inc.
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

package pkg

import (
	"testing"
	"github.com/consensusdb/consensusdb/pkg/pb"
	"fmt"
	"github.com/consensusdb/timeuuid"
	"math/rand"
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

	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetTime(time.Now())
	uuid.SetCounter(rand.Int63())

	fmt.Print("timbased uuid=", uuid.String(), "\n")

	key.Timestamp = &pb.TimeUUID{ MostSigBits:uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }

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
