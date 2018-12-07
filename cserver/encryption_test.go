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

package cserver_test

import (
	"testing"
	"github.com/consensusdb/consensusdb/cserver"
	"bytes"
	"reflect"
	"crypto/cipher"
	"github.com/consensusdb/consensusdb/c"
)

var KeySizes = [...]int{128, 192, 256}

func TestEncryption(t *testing.T) {

	hash, err := cserver.GetPasswordHash("test")
	if err != nil {
		t.Fatal("fail to hash password", err)
	}

	for _, keySize := range KeySizes {

		key := cserver.GetCipherKey(hash, keySize / 8)

		for _, c := range cserver.KnownCiphers {

			block, err := c.Create(key)
			if err != nil {
				t.Fatal("fail to create cipher", err)
			}

			for _, mode := range cserver.KnownCipherModes {

				RunCipherTest(t, mode, block)

			}

		}



	}

}

func RunCipherTest(t *testing.T, mode cserver.ICipherMode, block cipher.Block) {

	original := []byte("alexshvid")

	plaintext := c.CopyOf(original)
	ciphertext, err := mode.Encrypt(block, plaintext)

	if !bytes.Equal(plaintext, original) {
		t.Fatal("plaintext must not be modified", err, " for ", reflect.TypeOf(mode))
	}

	if err != nil {
		t.Fatal("fail to encrypt", err, " for ", reflect.TypeOf(mode))
	}

	keepCiphertext := c.CopyOf(ciphertext)
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

