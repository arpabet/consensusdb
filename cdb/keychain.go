/*
 *
 * Copyright 2020-present Arpabet, Inc.
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
	"github.com/pkg/errors"
	"github.com/consensusdb/timeuuid"
	"crypto/sha256"
)

var (
	ErrorInvalidKeyLength = errors.New("invalid key length")
)

type BlockKey []byte

type Keychain interface {

	GetBlockKey(majorKey []byte, timestamp timeuuid.UUID, keyLenBits int) (BlockKey, error)

}

type PasswordbasedKeychain struct {

	passwordHash [32]byte

}

func NewPasswordbasedKeychain(password string) (kc *PasswordbasedKeychain, err error) {
	kc = &PasswordbasedKeychain { passwordHash: sha256.Sum256([]byte(password)) }
	return kc, nil
}

func (this*PasswordbasedKeychain) GetBlockKey(majorKey []byte, timestamp timeuuid.UUID, keyLenBits int) (BlockKey, error) {

	keyLenBytes := keyLenBits / 8

	if keyLenBytes > len(this.passwordHash) {
		return nil, ErrorInvalidKeyLength
	}

	return CopyOf(this.passwordHash[:keyLenBytes]), nil

}

func (b BlockKey) clear() {
	size := len(b)
	for i := 0; i < size; i = i + 1 {
		b[i] = 0
	}
}




