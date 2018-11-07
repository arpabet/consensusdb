/*
 *
 * Copyright 2018-present Alexander Shvid and other authors
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

type ISecurity interface {

	GetEncryptionKey(topo string, timestamp uint64, len int) ([]byte, error)

}

type SimpleSecurityContext struct {

	passwordHash   []byte

}

func NewSimpleSecurityContext(password string) (context *SimpleSecurityContext, err error) {

	context = new(SimpleSecurityContext)

	context.passwordHash, err = GetPasswordHash(password)

	return context, err
}

func (this* SimpleSecurityContext) GetEncryptionKey(topo string, timestamp uint64, len int) ([]byte, error) {

	return this.passwordHash[:len], nil

}
