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
)

type ICompression interface {

	Compress([]byte, bbproto.CompressionLevel) ([]byte, error)

	Decompress([]byte) ([]byte, error)

}

var KnownCompressions = map[bbproto.CompressionAlgorithm]ICompression {
	bbproto.CompressionAlgorithm_DC_NO: &NoCompression{},
	bbproto.CompressionAlgorithm_DC_FLATE: &FlateCompression{},
	bbproto.CompressionAlgorithm_DC_GZIP: &GZIPCompression{},
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

//
//  Flate Compression
//

type FlateCompression struct {
}

func FlateCommpressionLevel(level bbproto.CompressionLevel) int {

	switch level {

	case bbproto.CompressionLevel_BEST_SPEED:
		return flate.BestSpeed

	case bbproto.CompressionLevel_BEST_COMPRESSION:
		return flate.BestCompression

	}

	return flate.DefaultCompression;
}

func (this* FlateCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

	var b bytes.Buffer

	w, err := flate.NewWriter(&b, FlateCommpressionLevel(level))
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

func GZIPCommpressionLevel(level bbproto.CompressionLevel) int {

	switch level {

	case bbproto.CompressionLevel_BEST_SPEED:
		return gzip.BestSpeed

	case bbproto.CompressionLevel_BEST_COMPRESSION:
		return gzip.BestCompression

	}

	return gzip.DefaultCompression;
}

func (this* GZIPCompression) Compress(input []byte, level bbproto.CompressionLevel) (output []byte, err error) {

	var b bytes.Buffer

	w, err := gzip.NewWriterLevel(&b, GZIPCommpressionLevel(level))
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

