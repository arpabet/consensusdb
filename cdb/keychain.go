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

package cdb

import (
	"golang.org/x/crypto/bcrypt"
	"github.com/pkg/errors"
	"github.com/shvid/timeuuid"
)

var (
	ErrorInvalidKeyLength = errors.New("invalid key length")
)

type Keychain interface {

	GetBlockKey(majorKey []byte, timestamp timeuuid.UUID, keyLenBits int) ([]byte, error)

}

type PasswordbasedKeychain struct {

	passwordHash []byte

}

func NewPasswordbasedKeychain(password string) (kc *PasswordbasedKeychain, err error) {
	kc = new(PasswordbasedKeychain)
	kc.passwordHash, err = getPasswordHash(password)
	return
}

func (this*PasswordbasedKeychain) GetBlockKey(majorKey []byte, timestamp timeuuid.UUID, keyLenBits int) ([]byte, error) {

	keyLenBytes := keyLenBits / 8

	if keyLenBytes > len(this.passwordHash) {
		return nil, ErrorInvalidKeyLength
	}

	return this.passwordHash[:keyLenBytes], nil

}

func getPasswordHash(password string) ([]byte, error) {

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return hash, nil

}



