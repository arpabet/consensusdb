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

import (
	"golang.org/x/crypto/bcrypt"
)

type ISecurityContext interface {

	GetEncryptionKey(majorKey []byte, timestamp uint64, keyLen int) ([]byte, error)

}

type SimpleSecurityContext struct {

	hash  []byte

}

func NewSimpleSecurityContext(password string) (context *SimpleSecurityContext, err error) {

	context = new(SimpleSecurityContext)

	context.hash, err = GetPasswordHash(password)

	return
}

func (this* SimpleSecurityContext) GetEncryptionKey(majorKey []byte, timestamp uint64, keyLen int) ([]byte, error) {

	return this.hash[:keyLen], nil

}

func GetPasswordHash(password string) ([]byte, error) {

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return hash, nil

}

func GetCipherKey(hash []byte, keyLen int) ([]byte) {
	return hash[:keyLen]
}


