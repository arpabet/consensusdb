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

package bbserver

import (
	"compress/gzip"
	"bytes"
	"io/ioutil"
	"compress/flate"
	"compress/lzw"
	"compress/zlib"
	"github.com/dsnet/compress/bzip2"
	"github.com/pierrec/lz4"
)

type ICompressor interface {

	Compress(payload []byte, level int) ([]byte, error)

	Decompress(payload []byte) ([]byte, error)

}

var KnownCompressors = map[string]ICompressor {
	"FLATE": &FlateCompressor{},
	"GZIP": &GZIPCompressor{},
	"LZW": &LZWCompressor{},
	"ZLIB": &ZLIBCompressor{},
	"BZIP2": &BZIP2Compressor{},
	"LZ4": &LZ4Compressor{},
}

type NoCompressor struct {
}

func (this*NoCompressor) Compress(input []byte, level int) (output []byte, err error) {
	return input, nil
}

func (this*NoCompressor) Decompress(input  []byte) (output []byte, err error) {
	return input, nil
}

func FlateCompressionLevel(level int) int {

	if level == 6 {
		return -1
	}

	return level
}

func BZIP2CompressionLevel(level int) int {
	return level
}

//
//  Flate Compressor
//

type FlateCompressor struct {
}

func (this*FlateCompressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	w, err := flate.NewWriter(&b, FlateCompressionLevel(level))
	defer w.Close()

	if err != nil {
		return nil, err
	}

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*FlateCompressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r := flate.NewReader(b)
	defer r.Close()

	return ioutil.ReadAll(r)

}


//
//  GZIP Compressor
//

type GZIPCompressor struct {
}

func (this*GZIPCompressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	w, err := gzip.NewWriterLevel(&b, FlateCompressionLevel(level))
	defer w.Close()

	if err != nil {
		return nil, err
	}

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*GZIPCompressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r, err := gzip.NewReader(b)
	defer r.Close()

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}


//
//  LZW Compressor
//

type LZWCompressor struct {
}

func (this*LZWCompressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	w := lzw.NewWriter(&b, lzw.LSB, 8)
	defer w.Close()

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*LZWCompressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r := lzw.NewReader(b, lzw.LSB, 8)
	defer r.Close()

	return ioutil.ReadAll(r)

}

//
//  ZLIB Compressor
//

type ZLIBCompressor struct {
}

func (this*ZLIBCompressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	w, err := zlib.NewWriterLevel(&b, FlateCompressionLevel(level))
	defer w.Close()

	if err != nil {
		return nil, err
	}

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*ZLIBCompressor) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r, err := zlib.NewReader(b)
	defer r.Close()

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}


//
//  BZIP2 Compressor
//

type BZIP2Compressor struct {
}

func (this*BZIP2Compressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	config := bzip2.WriterConfig { Level: BZIP2CompressionLevel(level) }

	w, err := bzip2.NewWriter(&b, &config)

	if err != nil {
		return nil, err
	}

	if _, err := w.Write(input); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (this*BZIP2Compressor) Decompress(input  []byte) (output []byte, err error) {

	var config bzip2.ReaderConfig

	b := bytes.NewBuffer(input)
	r, err := bzip2.NewReader(b, &config)

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}

//
//  LZ4 Compressor
//

type LZ4Compressor struct {
}

func (this*LZ4Compressor) Compress(input []byte, level int) (output []byte, err error) {

	var b bytes.Buffer

	w := lz4.NewWriter(&b)
	w.CompressionLevel = level

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