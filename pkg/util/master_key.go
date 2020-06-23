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

package util

import (
	"crypto/rand"
	"github.com/consensusdb/consensusdb/pkg/constants"
	"github.com/pkg/errors"
	"io"
)


func GenerateMasterKey() (string, error) {
	nonce := make([]byte, constants.KeySize)
	if _, err := io.ReadFull(rand.Reader, nonce); err == nil {
		key := constants.Encoding.EncodeToString(nonce)
		return key, nil
	} else {
		return "", err
	}
}

func ParseMasterKey(base64key string) ([]byte, error) {
	key, err := constants.Encoding.DecodeString(base64key)
	if err != nil {
		return key, err
	}
	if len(key) != constants.KeySize {
		return key, errors.Errorf("wrong key size %d", len(key))
	}
	return key, nil
}

