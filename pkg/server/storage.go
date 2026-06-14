/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/pb"
	badger "github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"time"
)


const (
	defBlockSize = 100
)

type BlockSender interface {

	Send(*pb.Block) error

}

type KeyValueStorage interface {

	Get(keyRequest *pb.KeyRequest) (*pb.Record, error);

	GetRecent(keyRequest *pb.KeyRequest) (*pb.Record, error);

	GetRange(rangeRequest *pb.RangeRequest) (*pb.Block, error);

	GetArea(keyRequest *pb.KeyRequest, lastField Field, sender BlockSender) error;

	Scan(scanRequest *pb.ScanRequest, sender BlockSender) error;

	Touch(recordRequest *pb.RecordRequest) (*pb.Status, error);

	Put(recordRequest *pb.RecordRequest) (*pb.Status, error);

	Remove(keyRequest *pb.KeyRequest) (*pb.Status, error);

	Close() error

}

type KeyValueStorageCtx struct {

	db        			*badger.DB
	conf      			*Configuration
	log                 *zap.Logger

}

func OpenKeyValueStorage(conf *Configuration, log *zap.Logger) (context *KeyValueStorageCtx, err error) {

	context = &KeyValueStorageCtx{conf: conf, log: log}

	opts := badger.DefaultOptions(conf.DataDir)
	opts.Dir = conf.KeyDir
	opts.ValueDir = conf.ValueDir
	if conf.FileIO {
		opts.TableLoadingMode = options.FileIO
		opts.ValueLogLoadingMode = options.FileIO
	}
	opts.CompactL0OnClose = true
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

func FetchRecord(key *pb.Key, item *badger.Item, headOnly bool) (*pb.Record, error) {

	if headOnly {
		return RecordNotFetched(key, item), nil
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, errors.Errorf("fetch value failed: %v", err)
	}

	return RecordFetched(key, item, data), nil

}

func (this *KeyValueStorageCtx) Get(keyRequest *pb.KeyRequest) (*pb.Record, error) {

	if keyRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(keyRequest.Key)

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(entryKey)
	if err != nil {

		if err == badger.ErrKeyNotFound {
			return RecordNotFound(keyRequest.Key), nil
		} else {
			return nil, err
		}

	}

	return FetchRecord(keyRequest.Key, item, keyRequest.HeadOnly)
}

func (this *KeyValueStorageCtx) GetRecent(keyRequest *pb.KeyRequest) (*pb.Record, error) {

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
		item := iter.Item()
		actualKey, err := DecodeKey(item.Key())
		if err != nil {
			return nil, err
		}
		return FetchRecord(actualKey, iter.Item(), keyRequest.HeadOnly);
	} else {
		return RecordNotFound(keyRequest.Key), nil
	}

}

func (this *KeyValueStorageCtx) GetRange(rangeRequest *pb.RangeRequest) (*pb.Block, error) {

	if rangeRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	if rangeRequest.Type != pb.RangeType_LESS_OR_EQUAL {
		return nil, ErrorUnsupportedOperation
	}

	size := int(rangeRequest.NumRecords)

	if size <= 0 {
		return nil, ErrorWrongSize
	}

	records := make([]*pb.Record, 0, size)

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

	return &pb.Block { Record: records }, nil

}

func (this *KeyValueStorageCtx) GetArea(keyRequest *pb.KeyRequest, lastField Field, sender BlockSender) error {

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

	records := make([]*pb.Record, 0, defBlockSize)
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

			err = sender.Send( &pb.Block{ Record: records } )
			if err != nil {
				return err
			}

			records = make([]*pb.Record, 0, defBlockSize)

		}

	}

	if len(records) > 0 {
		err = sender.Send( &pb.Block{ Record: records } )
	}

	return err

}


func (this *KeyValueStorageCtx) Scan(scanRequest *pb.ScanRequest, sender BlockSender) error {

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

	iter.Rewind()

	records := make([]*pb.Record, 0, defBlockSize)
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

			err = sender.Send( &pb.Block{ Record: records } )
			if err != nil {
				return err
			}

			records = make([]*pb.Record, 0, defBlockSize)

		}

	}

	if len(records) > 0 {
		err := sender.Send( &pb.Block{ Record: records } )
		if err != nil {
			return err
		}
	}

	return nil
}

func (this *KeyValueStorageCtx) Touch(recordRequest *pb.RecordRequest) (*pb.Status, error) {

	if recordRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(recordRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get(entryKey)

	if err != nil {

		if err == badger.ErrKeyNotFound {
			return &pb.Status{ Updated: false}, nil
		} else {
			return nil, err
		}

	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, errors.Errorf("fetch in touch error: %v", err)
	}

	entry := &badger.Entry{ Key: entryKey, Value:data, UserMeta: item.UserMeta()  }

	if recordRequest.TtlSeconds > 0 {
		ttl := time.Duration(recordRequest.TtlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return nil, errors.Errorf("update entry error: %v", err)
	}

	txn.Commit()

	return &pb.Status{ Updated: true}, nil

}

func (this *KeyValueStorageCtx) Put(recordRequest *pb.RecordRequest) (*pb.Status, error) {

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
				return &pb.Status{ Updated: false}, nil
			}

			// putIfAbsent

		} else if recordRequest.Version != item.Version() {

			// wrong version CAS
			return &pb.Status{ Updated: false}, nil
		}

	}

	entry := &badger.Entry{ Key: entryKey, Value: recordRequest.Value, UserMeta: byte(recordRequest.Metadata) }

	if recordRequest.TtlSeconds > 0 {
		ttl := time.Duration(recordRequest.TtlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err := txn.SetEntry(entry)

	if err != nil {
		return nil, errors.Errorf("update entry error: %v", err)
	}

	txn.Commit()

	return &pb.Status{ Updated: true}, nil

}

func (this *KeyValueStorageCtx) Remove(keyRequest *pb.KeyRequest) (*pb.Status, error) {

	if keyRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(keyRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	err := txn.Delete(entryKey)

	if err != nil {
		return nil, errors.Errorf("delete entry error: %v", err)
	}

	txn.Commit()

	return &pb.Status{ Updated: true}, nil

}