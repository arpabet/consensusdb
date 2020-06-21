/*
 *
 * Copyright 2020-present Arpabet Inc.
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

package cdb

import (
	"testing"
	"bytes"
	"reflect"
	"crypto/cipher"
	"github.com/consensusdb/timeuuid"
	"time"
	"math/rand"
)


func TestEncryption(t *testing.T) {

	input := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		input[i] = byte(i)
	}

	keychain, err := NewPasswordbasedKeychain("test")

	if err != nil {
		t.Fatal("fail to passwordHash password", err)
	}

	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetTime(time.Now())
	uuid.SetCounter(rand.Int63())

	for _, c := range KnownCiphers {

		key, err := keychain.GetBlockKey([]byte("majorKey"), uuid, c.KeyLengthBits())
		if err != nil {
			t.Fatal("fail to get key", err)
		}
		defer key.clear()

		block, err := c.Create(key)
		if err != nil {
			t.Fatal("fail to create cipher", err)
		}

		for _, mode := range KnownCipherModes {

			RunCipherTest(t, mode, block, []byte(""))
			RunCipherTest(t, mode, block, []byte("a"))
			RunCipherTest(t, mode, block, []byte("alex"))
			RunCipherTest(t, mode, block, input)

		}

	}


}

func RunCipherTest(t *testing.T, mode CipherMode, block cipher.Block, original []byte) {


	plaintext := CopyOf(original)
	ciphertext, err := mode.Encrypt(block, plaintext)

	if !bytes.Equal(plaintext, original) {
		t.Fatal("plaintext must not be modified", err, " for ", reflect.TypeOf(mode))
	}

	if err != nil {
		t.Fatal("fail to encrypt", err, " for ", reflect.TypeOf(mode))
	}

	keepCiphertext := CopyOf(ciphertext)
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

