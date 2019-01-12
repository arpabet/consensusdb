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
	"bytes"
	"io/ioutil"
	"github.com/pierrec/lz4"
	"github.com/golang/snappy"
)

type ICompressor interface {
	MetadataFlag() byte
	Compress(input []byte) ([]byte, error)
	Decompress(input []byte) ([]byte, error)
}

var (
	LZ4 = &LZ4Compressor{}
	SNAPPY = &SnappyCompressor{}
)

var KnownCompressors = map[string]ICompressor {
	"no": &NoCompressor{},
	"lz4": LZ4,
	"lz4_high": &LZ4HighCompressor{},
	"snappy": SNAPPY,
}

type NoCompressor struct {
}

func (this*NoCompressor) MetadataFlag() byte {
	return 0
}

func (this*NoCompressor) Compress(input []byte) (output []byte, err error) {
	return input, nil
}

func (this*NoCompressor) Decompress(input  []byte) (output []byte, err error) {
	return input, nil
}

//
//  LZ4 Compressor
//

type LZ4Compressor struct {
}

func (this*LZ4Compressor) MetadataFlag() byte {
	return bitLZ4
}

func (this*LZ4Compressor) Compress(input []byte) (output []byte, err error) {

	var b bytes.Buffer

	w := lz4.NewWriter(&b)

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*LZ4Compressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)
	r := lz4.NewReader(b)

	return ioutil.ReadAll(r)

}

//
//  LZ4 Compressor High
//

type LZ4HighCompressor struct {
}

func (this*LZ4HighCompressor) MetadataFlag() byte {
	return bitLZ4
}

func (this*LZ4HighCompressor) Compress(input []byte) (output []byte, err error) {

	var b bytes.Buffer

	w := lz4.NewWriter(&b)
	w.CompressionLevel = 9

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*LZ4HighCompressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)
	r := lz4.NewReader(b)

	return ioutil.ReadAll(r)

}

//
//  Snappy Compressor
//

type SnappyCompressor struct {
}

func (this*SnappyCompressor) MetadataFlag() byte {
	return bitSnappy
}

func (this*SnappyCompressor) Compress(input []byte) (output []byte, err error) {
	return snappy.Encode(nil, input), nil;
}

func (this*SnappyCompressor) Decompress(input []byte) (output []byte, err error) {
	return snappy.Decode(nil, input)
}