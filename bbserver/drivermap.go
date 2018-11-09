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

import "sync"

type DriverMap struct {
	sync.RWMutex
	internal map[string]IDriver
}

type DriverEntry struct {
	Key    string
	Value  IDriver
}

func NewDriverMap() *DriverMap {
	return &DriverMap{
		internal: make(map[string]IDriver),
	}
}

func (this *DriverMap) Get(key string) (result IDriver, ok bool) {
	this.RLock()
	result, ok = this.internal[key]
	this.RUnlock()
	return result, ok
}

func (this *DriverMap) Put(key string, value IDriver) {
	this.Lock()
	this.internal[key] = value
	this.Unlock()
}

func (this *DriverMap) Remove(key string) (prev IDriver, ok bool) {
	this.Lock()
	prev, ok = this.internal[key]
	delete(this.internal, key)
	this.Unlock()
	return prev, ok
}

func (this *DriverMap) Clear() {
	this.Lock()
	this.internal = make(map[string]IDriver)
	this.Unlock()
}

func (this *DriverMap) List() []DriverEntry {
	clone := make([]DriverEntry, 0, len(this.internal))
	this.RLock()
	for k, v := range this.internal {
		clone = append(clone, DriverEntry{ k, v })
	}
	this.RUnlock()
	return clone
}