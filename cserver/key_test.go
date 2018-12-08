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
	"testing"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"fmt"
	"bytes"
)

func TestKey(t *testing.T) {

	//
	// No Timestamp
	//

	key := &cserverpb.Key{MajorKey: []byte("ashvid"), RegionName: "accounts", MinorKey: []byte("Bank1")}

	size := GetEncodedSize(key)

	if size != 2 + 6 + 2 + 8 + 2 + 5 {
		t.Fatal("wrong encoded size")
	}

	b := make([]byte, size)
	prefixLen := EncodeKey(key, b)

	if prefixLen != 2 + 6 + 2 + 8 + 2 + 5 {
		t.Fatal("wrong prefix len")
	}

	fmt.Print("No timestamp: ", b, "\n")

	buf := &bytes.Buffer{}
	EncodeKeyTo(key, buf)

	if !bytes.Equal(b, buf.Bytes()) {
		t.Fatal("wrong encoded to")
	}

	other := DecodeKey(b)

	if !IsEquals(key, other) {
		t.Fatal("wrong decode")
	}


	//
	// With Timestamp
	//

	key.Timestamp = 123

	size = GetEncodedSize(key)

	if size != 2 + 6 + 2 + 8 + 2 + 5 + 8 {
		t.Fatal("wrong encoded size with timestamp")
	}

	b = make([]byte, size)
	prefixLen = EncodeKey(key, b)

	if prefixLen != 2 + 6 + 2 + 8 + 2 + 5 {
		t.Fatal("wrong prefix len with timestamp")
	}

	fmt.Print("With timestamp: ", b, "\n")

	buf = &bytes.Buffer{}
	EncodeKeyTo(key, buf)

	if !bytes.Equal(b, buf.Bytes()) {
		t.Fatal("wrong encoded to with timestamp")
	}

	other = DecodeKey(b)

	if !IsEquals(key, other) {
		t.Fatal("wrong decode with timestamp")
	}



}
