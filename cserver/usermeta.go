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

const (
	bitCompressed             byte = 1 << 0    // Set if the entry has been compressed.
	bitEncrypted              byte = 1 << 1    // Set if the entry has been encrypted.
	bitChunked                byte = 1 << 2    // Set if the entry was packed in to the chunk.
)

func isCompressionEnabled(userMeta byte) bool {
	return userMeta & bitCompressed > 0
}

func SetCompressionEnabled(userMeta byte) byte {
	return userMeta | bitCompressed
}

func isEncryptionEnabled(userMeta byte) bool {
	return userMeta & bitEncrypted > 0
}

func SetEncryptionEnabled(userMeta byte) byte {
	return userMeta | bitEncrypted
}

func isChunked(userMeta byte) bool {
	return userMeta & bitChunked > 0
}

func SetChunked(userMeta byte) byte {
	return userMeta | bitChunked
}

