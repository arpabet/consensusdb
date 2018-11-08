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

package bbserver

import "github.com/pkg/errors"

type ISecurity interface {

	GetEncryptionKey(topo string, timestamp uint64, len int) ([]byte, error)

}

type SimpleSecurityContext struct {

	hashMap   map[string][]byte

}

func NewSimpleSecurityContext(passwordMap map[string]string) (context *SimpleSecurityContext, err error) {

	context = &SimpleSecurityContext{hashMap: make(map[string][]byte)}

	for topo, password := range passwordMap {

		context.hashMap[topo], err = GetPasswordHash(password)

		if err != nil {
			return nil, err
		}
	}

	return context, nil
}

func (this* SimpleSecurityContext) GetEncryptionKey(topo string, timestamp uint64, len int) ([]byte, error) {

	hash, ok := this.hashMap[topo]

	if !ok {
		return nil, errors.New("topo not found: " + topo)
	}

	return hash[:len], nil

}
