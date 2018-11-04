/*
 *
 * Copyright 2018-present Alexander Shvid and other authors
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
	"bigbagger/proto/bbproto"
	"compress/gzip"
	"bytes"
	"io/ioutil"
	"compress/flate"
	"compress/lzw"
	"compress/zlib"
	"github.com/dsnet/compress/bzip2"
)

type ICompression interface {

	Compress([]byte, bbproto.CompressionLevel) ([]byte, error)

	Decompress([]byte) ([]byte, error)

}

var KnownCompressions = map[bbproto.CompressionAlgorithm]ICompression {
	bbproto.CompressionAlgorithm_COMPRESS_NO: &NoCompression{},
	bbproto.CompressionAlgorithm_COMPRESS_FLATE: &FlateCompression{},
	bbproto.CompressionAlgorithm_COMPRESS_GZIP: &GZIPCompression{},
	bbproto.CompressionAlgorithm_COMPRESS_LZW: &LZWCompression{},
	bbproto.CompressionAlgorithm_COMPRESS_ZLIB: &ZLIBCompression{},
	bbproto.CompressionAlgorithm_COMPRESS_BZIP2: &BZIP2Compression{},
}

//
//  No Compression
//

type NoCompression struct {
}

func (this* NoCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {
	return input, nil
}

func (this* NoCompression) Decompress(input  []byte) (output []byte, err error) {
	return input, nil
}

func FlateCompressionLevel(level bbproto.CompressionLevel) int {

	switch level {

	case bbproto.CompressionLevel_BEST_SPEED:
		return 1

	case bbproto.CompressionLevel_BEST_COMPRESSION:
		return 9

	}

	return -1;
}

func BZIP2CompressionLevel(level bbproto.CompressionLevel) int {

	switch level {

	case bbproto.CompressionLevel_BEST_SPEED:
		return 1

	case bbproto.CompressionLevel_BEST_COMPRESSION:
		return 9

	}

	return 6;
}

//
//  Flate Compression
//

type FlateCompression struct {
}

func (this* FlateCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

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

func (this* FlateCompression) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r := flate.NewReader(b)
	defer r.Close()

	return ioutil.ReadAll(r)

}


//
//  GZIP Compression
//

type GZIPCompression struct {
}

func (this* GZIPCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

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

func (this* GZIPCompression) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r, err := gzip.NewReader(b)
	defer r.Close()

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}


//
//  LZW Compression
//

type LZWCompression struct {
}

func (this* LZWCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

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

func (this* LZWCompression) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r := lzw.NewReader(b, lzw.LSB, 8)
	defer r.Close()

	return ioutil.ReadAll(r)

}

//
//  ZLIB Compression
//

type ZLIBCompression struct {
}

func (this* ZLIBCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

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

func (this* ZLIBCompression) Decompress(input  []byte) (output []byte, err error) {

	b := bytes.NewBuffer(input)

	r, err := zlib.NewReader(b)
	defer r.Close()

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}


//
//  BZIP2 Compression
//

type BZIP2Compression struct {
}

func (this* BZIP2Compression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

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

func (this* BZIP2Compression) Decompress(input  []byte) (output []byte, err error) {

	var config bzip2.ReaderConfig

	b := bytes.NewBuffer(input)
	r, err := bzip2.NewReader(b, &config)

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)

}