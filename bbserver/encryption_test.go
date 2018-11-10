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
	"reflect"
	"crypto/cipher"
	"github.com/bigbagger/bigbagger/bbcommon"
)

func TestEncryptions(t *testing.T) {

	hash, err := bbserver.GetPasswordHash("test")
	if err != nil {
		t.Fatal("fail to hash password", err)
	}

	for len, _ := range bbproto.BlockSize_name {

		if len > 0 {

			key := bbserver.GetBlockKey(hash, len)

			for _, c := range bbserver.KnownCiphers {

				block, err := c.Create(key)
				if err != nil {
					t.Fatal("fail to create cipher", err)
				}

				for _, mode := range bbserver.KnownBlockModes {

					RunCipherTest(t, mode, block)

				}

			}

		}

	}

}

func RunCipherTest(t *testing.T, mode bbserver.IBlockMode, block cipher.Block) {

	original := []byte("alexshvid")

	plaintext := bbcommon.CopyOf(original)
	ciphertext, err := mode.Encrypt(block, plaintext)

	if !bytes.Equal(plaintext, original) {
		t.Fatal("plaintext must not be modified", err, " for ", reflect.TypeOf(mode))
	}

	if err != nil {
		t.Fatal("fail to encrypt", err, " for ", reflect.TypeOf(mode))
	}

	keepCiphertext := bbcommon.CopyOf(ciphertext)
	actual, err := mode.Decrypt(block, ciphertext)

	if !bytes.Equal(ciphertext, keepCiphertext) {
		t.Fatal("ciphertext must not be modified", err, " for ", reflect.TypeOf(mode))
	}

	if err != nil {
		t.Fatal("fail to decrypt", err, " for ", reflect.TypeOf(mode))
	}

	if !bytes.Equal(plaintext, actual) {
		t.Fatal("actual not the same as input", err, " for ", reflect.TypeOf(mode))
	}
}

