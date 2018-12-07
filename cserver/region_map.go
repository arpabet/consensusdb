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

import "sync"

type RegionStoreMap struct {
	sync.RWMutex
	internal map[string]IRegionStore
}

type RegionStoreEntry struct {
	Name    string
	Value   IRegionStore
}

func NewRegionStoreMap() *RegionStoreMap {
	return &RegionStoreMap{
		internal: make(map[string]IRegionStore),
	}
}

func (this *RegionStoreMap) Get(key string) (result IRegionStore, ok bool) {
	this.RLock()
	result, ok = this.internal[key]
	this.RUnlock()
	return result, ok
}

func (this *RegionStoreMap) Put(key string, value IRegionStore) {
	this.Lock()
	this.internal[key] = value
	this.Unlock()
}

func (this *RegionStoreMap) Remove(key string) (prev IRegionStore, ok bool) {
	this.Lock()
	prev, ok = this.internal[key]
	delete(this.internal, key)
	this.Unlock()
	return prev, ok
}

func (this *RegionStoreMap) Clear() {
	this.Lock()
	this.internal = make(map[string]IRegionStore)
	this.Unlock()
}

func (this *RegionStoreMap) List() []RegionStoreEntry {
	clone := make([]RegionStoreEntry, 0, len(this.internal))
	this.RLock()
	for k, v := range this.internal {
		clone = append(clone, RegionStoreEntry{ k, v })
	}
	this.RUnlock()
	return clone
}