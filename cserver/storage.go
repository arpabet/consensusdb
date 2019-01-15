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
	"github.com/dgraph-io/badger"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"time"
)


const (
	defBlockSize = 100
)

type BlockSender interface {

	Send(*cserverpb.Block) error

}

type KeyValueStorage interface {

	Get(keyRequest *cserverpb.KeyRequest) (*cserverpb.Record, error);

	GetRecent(keyRequest *cserverpb.KeyRequest) (*cserverpb.Record, error);

	GetRange(rangeRequest *cserverpb.RangeRequest) (*cserverpb.Block, error);

	GetArea(keyRequest *cserverpb.KeyRequest, lastField Field, sender BlockSender) error;

	Scan(scanRequest *cserverpb.ScanRequest, sender BlockSender) error;

	Touch(recordRequest *cserverpb.RecordRequest) (*cserverpb.Status, error);

	Put(recordRequest *cserverpb.RecordRequest) (*cserverpb.Status, error);

	Remove(keyRequest *cserverpb.KeyRequest) (*cserverpb.Status, error);

	Close() error

}

type KeyValueStorageCtx struct {

	db        			*badger.DB
	conf      			*Configuration
	log                 *zap.Logger

}

func OpenKeyValueStorage(conf *Configuration, log *zap.Logger) (context *KeyValueStorageCtx, err error) {

	context = &KeyValueStorageCtx{conf: conf, log: log}

	opts := badger.DefaultOptions
	opts.Dir = conf.KeyDir
	opts.ValueDir = conf.ValueDir
	context.db, err = badger.Open(opts)

	return context, err

}

func (this *KeyValueStorageCtx) Close() error {
	if this != nil && this.db != nil {
		this.log.Info("KV closing.")
		return this.db.Close()
	}
	return nil
}


func (this *KeyValueStorageCtx) Get(keyRequest *cserverpb.KeyRequest) (*cserverpb.Record, error) {

	if keyRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(keyRequest.Key)

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(entryKey)
	if err != nil {
		return RecordNotFound(keyRequest.Key), nil
	}

	return FetchRecord(keyRequest.Key, item, keyRequest.HeadOnly)
}

func FetchRecord(key *cserverpb.Key, item *badger.Item, headOnly bool) (*cserverpb.Record, error) {

	if headOnly {
		return RecordNotFetched(key, item), nil
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, errors.Errorf("fetch value failed: ", err)
	}

	return RecordFetched(key, item, data), nil

}

func (this *KeyValueStorageCtx) GetRecent(keyRequest *cserverpb.KeyRequest) (*cserverpb.Record, error) {

	if keyRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, rowKey := EncodeKey(keyRequest.Key)

	options := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   1,
		Reverse:        true,
		AllVersions:    false,
		Prefix: 		rowKey,
	}

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	iter := txn.NewIterator(options)
	defer iter.Close()

	iter.Seek(entryKey)

	if iter.Valid() {
		return FetchRecord(keyRequest.Key, iter.Item(), keyRequest.HeadOnly);
	} else {
		return RecordNotFound(keyRequest.Key), nil
	}

}

func (this *KeyValueStorageCtx) GetRange(rangeRequest *cserverpb.RangeRequest) (*cserverpb.Block, error) {

	if rangeRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	if rangeRequest.Type != cserverpb.RangeType_LESS_OR_EQUAL {
		return nil, ErrorUnsupportedOperation
	}

	size := int(rangeRequest.NumRecords)

	if size <= 0 {
		return nil, ErrorWrongSize
	}

	records := make([]*cserverpb.Record, 0, size)

	entryKey, rowKey := EncodeKey(rangeRequest.Key)

	options := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   size,
		Reverse:        true,
		AllVersions:    false,
		Prefix: 		rowKey,
	}

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	iter := txn.NewIterator(options)
	defer iter.Close()

	iter.Seek(entryKey)

	for i := 0; i < size && iter.Valid(); iter.Next() {

		item := iter.Item()
		itemKey, err := DecodeKey(item.Key())

		if err != nil {
			// ignore invalid key
			this.log.Error("decode key fail", zap.Error(err))
			continue
		}

		record, err := FetchRecord(itemKey, item, rangeRequest.HeadOnly);
		if err != nil {
			// ignore failed to fetch key
			this.log.Error("fetch record fail", zap.Error(err))
			continue
		}

		records = append(records, record)

		i = i + 1
	}

	return &cserverpb.Block { Record: records }, nil

}

func (this *KeyValueStorageCtx) GetArea(keyRequest *cserverpb.KeyRequest, lastField Field, sender BlockSender) error {

	if keyRequest.Key == nil {
		return ErrorEmptyKey
	}

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	keyPrefix, err := EncodeKeyPrefix(keyRequest.Key, lastField)
	if err != nil {
		return err
	}

	options := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   defBlockSize,
		Reverse:        false,
		AllVersions:    false,
		Prefix: 		keyPrefix,
	}

	iter := txn.NewIterator(options)
	defer iter.Close()

	iter.Seek(keyPrefix)

	records := make([]*cserverpb.Record, 0, defBlockSize)
	for ; iter.Valid(); iter.Next() {

		item := iter.Item()
		itemKey, err := DecodeKey(item.Key())

		if err != nil {
			// ignore invalid key
			this.log.Error("decode key fail", zap.Error(err))
			continue
		}

		record, err := FetchRecord(itemKey, item, keyRequest.HeadOnly);
		if err != nil {
			// ignore failed to fetch key
			this.log.Error("fetch record fail", zap.Error(err))
			continue
		}

		records = append(records, record)

		if len(records) == defBlockSize {

			err = sender.Send( &cserverpb.Block{ Record: records } )
			if err != nil {
				return err
			}

			records = make([]*cserverpb.Record, 0, defBlockSize)

		}

	}

	return nil

}


func (this *KeyValueStorageCtx) Scan(scanRequest *cserverpb.ScanRequest, sender BlockSender) error {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	options := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   defBlockSize,
		Reverse:        false,
		AllVersions:    false,
	}

	iter := txn.NewIterator(options)
	defer iter.Close()

	records := make([]*cserverpb.Record, 0, defBlockSize)
	for ; iter.Valid(); iter.Next() {

		item := iter.Item()
		itemKey, err := DecodeKey(item.Key())

		if err != nil {
			// ignore invalid key
			this.log.Error("decode key fail", zap.Error(err))
			continue
		}

		record, err := FetchRecord(itemKey, item, scanRequest.HeadOnly);
		if err != nil {
			// ignore failed to fetch key
			this.log.Error("fetch record fail", zap.Error(err))
			continue
		}

		records = append(records, record)

		if len(records) == defBlockSize {

			err = sender.Send( &cserverpb.Block{ Record: records } )
			if err != nil {
				return err
			}

			records = make([]*cserverpb.Record, 0, defBlockSize)

		}

	}

	return nil
}

func (this *KeyValueStorageCtx) Touch(recordRequest *cserverpb.RecordRequest) (*cserverpb.Status, error) {

	if recordRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(recordRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get(entryKey)

	if err != nil {
		// not found, not an error
		return &cserverpb.Status{ Updated: false}, nil
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, errors.Errorf("fetch in touch error: ", err)
	}

	entry := &badger.Entry{ Key: entryKey, Value:data, UserMeta: item.UserMeta()  }

	if recordRequest.TtlSeconds > 0 {
		ttl := time.Duration(recordRequest.TtlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return nil, errors.Errorf("update entry error: ", err)
	}

	return &cserverpb.Status{ Updated: true}, nil

}

func (this *KeyValueStorageCtx) Put(recordRequest *cserverpb.RecordRequest) (*cserverpb.Status, error) {

	if recordRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(recordRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	if recordRequest.CompareAndSet {

		item, err := txn.Get(entryKey)

		if err != nil {
			// absent

			if recordRequest.Version != 0 {
				return &cserverpb.Status{ Updated: false}, nil
			}

			// putIfAbsent

		} else if recordRequest.Version != item.Version() {

			// wrong version CAS
			return &cserverpb.Status{ Updated: false}, nil
		}

	}

	entry := &badger.Entry{ Key: entryKey, Value: recordRequest.Value }

	if recordRequest.TtlSeconds > 0 {
		ttl := time.Duration(recordRequest.TtlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err := txn.SetEntry(entry)

	if err != nil {
		return nil, errors.Errorf("update entry error: ", err)
	}

	return &cserverpb.Status{ Updated: true}, nil

}

func (this *KeyValueStorageCtx) Remove(keyRequest *cserverpb.KeyRequest) (*cserverpb.Status, error) {

	if keyRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(keyRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	err := txn.Delete(entryKey)

	if err != nil {
		return nil, errors.Errorf("delete entry error: ", err)
	}

	return &cserverpb.Status{ Updated: true}, nil

}