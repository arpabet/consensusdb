/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cdb

const (

	// first 8 bits are stored in userMeta, the 9-bit added by consensusdb from entry flags

	bitReserved1            int32 = 1 << 0
	bitReserved2            int32 = 1 << 1

	bitLZ4                  int32 = 1 << 2    // Set if the entry was compressed by LZ4
	bitSnappy               int32 = 1 << 3    // Set if the entry was compressed by Snappy

	bitAES      			int32 = 1 << 4    // Set if the entry was encrypted by AES-256 algorithm
	bitGCM					int32 = 1 << 5    // Set if the entry was encrypted by GCM cipher mode
	bitCFB					int32 = 1 << 6    // Set if the entry was encrypted by CFB cipher mode

	bitReserved3            int32 = 1 << 7
	bitReserved4            int32 = 1 << 8

	bitDeleted              int32 = 1 << 9    // Set if entry was deleted

)

func VerboseMetadata(metadata int32) string {

	str := make([]byte, 0, 32)

	if metadata & bitLZ4 > 0 {
		str = append(str, 'L','Z','4', ';')
	}

	if metadata & bitSnappy > 0 {
		str = append(str, 'S','n','a','p','p','y', ';')
	}

	if metadata & bitAES > 0 {
		str = append(str, 'A','E','S', ';')
	}

	if metadata & bitGCM > 0 {
		str = append(str, 'G','C','M', ';')
	}

	if metadata & bitCFB > 0 {
		str = append(str, 'C','F','B', ';')
	}

	return string(str)
}