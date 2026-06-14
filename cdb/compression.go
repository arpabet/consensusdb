/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cdb

import (
	"bytes"
	"io/ioutil"
	"github.com/pierrec/lz4"
	"github.com/golang/snappy"
)

type Compressor interface {

	MetadataFlag() int32
	Compress(input []byte) ([]byte, error)
	Decompress(input []byte) ([]byte, error)

}

var (

	NO_COMPRESSION = &NoCompression{}

	LZ4 = &LZ4Compressor{}
	LZ4_HIGH = &LZ4HighCompressor{}
	SNAPPY = &SnappyCompressor{}

	KnownCompressors = map[string]Compressor{
		"LZ4": LZ4,
		"LZ4_HIGH": LZ4_HIGH,
		"SNAPPY": SNAPPY,
	}

)

type NoCompression struct {
}

func (this*NoCompression) MetadataFlag() int32 {
	return 0
}

func (this*NoCompression) Compress(input []byte) (output []byte, err error) {
	return input, nil
}

func (this*NoCompression) Decompress(input  []byte) (output []byte, err error) {
	return input, nil
}

func (this *NoCompression) String() string {
	return "NO_COMPRESSION"
}

//
//  LZ4 Compressor
//

type LZ4Compressor struct {
}

func (this*LZ4Compressor) MetadataFlag() int32 {
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

func (this *LZ4Compressor) String() string {
	return "LZ4"
}

//
//  LZ4 Compressor High
//

type LZ4HighCompressor struct {
}

func (this*LZ4HighCompressor) MetadataFlag() int32 {
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

func (this *LZ4HighCompressor) String() string {
	return "LZ4_HIGH"
}

//
//  Snappy Compressor
//

type SnappyCompressor struct {
}

func (this*SnappyCompressor) MetadataFlag() int32 {
	return bitSnappy
}

func (this*SnappyCompressor) Compress(input []byte) (output []byte, err error) {
	return snappy.Encode(nil, input), nil;
}

func (this*SnappyCompressor) Decompress(input []byte) (output []byte, err error) {
	return snappy.Decode(nil, input)
}

func (this *SnappyCompressor) String() string {
	return "Snappy"
}