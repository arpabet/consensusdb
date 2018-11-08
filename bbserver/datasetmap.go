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

type DatasetMap struct {
	sync.RWMutex
	internal map[string]*DatasetContext
}

type DatasetEntry struct {
	Key    string
	Value  *DatasetContext
}

func NewDatasetMap() *DatasetMap {
	return &DatasetMap{
		internal: make(map[string]*DatasetContext),
	}
}

func (this *DatasetMap) Get(key string) (result *DatasetContext, ok bool) {
	this.RLock()
	result, ok = this.internal[key]
	this.RUnlock()
	return result, ok
}

func (this *DatasetMap) Put(key string, value *DatasetContext) {
	this.Lock()
	this.internal[key] = value
	this.Unlock()
}

func (this *DatasetMap) Remove(key string) (prev *DatasetContext, ok bool) {
	this.Lock()
	prev, ok = this.internal[key]
	delete(this.internal, key)
	this.Unlock()
	return prev, ok
}

func (this *DatasetMap) Clear() {
	this.Lock()
	this.internal = make(map[string]*DatasetContext)
	this.Unlock()
}

func (this *DatasetMap) List() []DatasetEntry {
	clone := make([]DatasetEntry, 0, len(this.internal))
	this.RLock()
	for k, v := range this.internal {
		clone = append(clone, DatasetEntry{ k, v })
	}
	this.RUnlock()
	return clone
}