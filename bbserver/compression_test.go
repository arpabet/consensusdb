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

package bbserver_test

import (
	"testing"
	"github.com/bigbagger/bigbagger/bbserver"
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"bytes"
	"fmt"
	"reflect"
)

func TestCompressions(t *testing.T) {

	input := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		input[i] = byte(i)
	}

	for _, v := range bbserver.KnownCompressors {

		for _, level := range bbproto.CompressionLevel_value {
			CompressionTest(t, input, v, bbproto.CompressionLevel(level))
		}

	}


}


func CompressionTest(t *testing.T, input []byte, compression bbserver.ICompressor, level bbproto.CompressionLevel) {

	output, err := compression.Compress(input, level)
	if err != nil {
		t.Fatal("fail to compress ", err)
	}

	fmt.Print("output.len=", len(output), " for = ", reflect.TypeOf(compression), " level=", level, "\n")

	actual, err := compression.Decompress(output)
	if err != nil {
		t.Fatal("fail to decompress ", err)
	}

	if !bytes.Equal(input, actual) {
		t.Fatal("actual not the same as input", err)
	}

}