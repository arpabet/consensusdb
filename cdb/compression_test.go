/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cdb

import (
	"testing"
	"bytes"
	"fmt"
	"reflect"
)


func TestCompression(t *testing.T) {

	input := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		input[i] = byte(i)
	}

	for _, v := range KnownCompressors {
		CompressorTest(t, input, v)
	}

}


func CompressorTest(t *testing.T, input []byte, compression Compressor) {

	output, err := compression.Compress(input)
	if err != nil {
		t.Fatal("fail to compress ", err)
	}

	fmt.Print("output.len=", len(output), " for = ", reflect.TypeOf(compression), "\n")

	actual, err := compression.Decompress(output)
	if err != nil {
		t.Fatal("fail to decompress ", err)
	}

	if !bytes.Equal(input, actual) {
		t.Fatal("actual not the same as input", err)
	}

}