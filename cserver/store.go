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
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"os"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/jsonpb"
	"github.com/consensusdb/consensusdb/c"
	"fmt"
	"time"
	"log"
	"github.com/pkg/errors"
	"math"
	"github.com/dgraph-io/badger"
)


type DefaultStore struct {

	regionName          string

	db        			*badger.DB
	region    			*cserverpb.Region
	regionDir 			string
	conf      			*Configuration

}

type DefaultStoreTxn struct {
	store   *DefaultStore
	update  bool
	txn     *badger.Txn
}

//
// IRegionTnx
//

func (this *DefaultStoreTxn) Update(update bool) {
	this.update = update
}

func (this *DefaultStoreTxn) Begin() {
	this.txn = this.store.db.NewTransaction(this.update)
}

func (this *DefaultStoreTxn) ProcessOperation(op *cserverpb.TxOperation) *cserverpb.TxOperationResult {
	return this.store.ProcessOperation(this.txn, op)
}

func (this *DefaultStoreTxn) Rollback() {
	this.txn.Discard()
}

func (this *DefaultStoreTxn) Commit() error {
	return this.txn.Commit()
}


//
// IRegionStore
//

func (this *DefaultStore) GetName() string {
	return this.regionName
}

func (this *DefaultStore) GetRegion() *cserverpb.Region {
	return this.region
}

func (this *DefaultStore) NewTransaction() IRegionTnx {
	return &DefaultStoreTxn{store:this}
}

func (this *DefaultStore) GetSnapshot(majorKey []byte, outC chan<- *cserverpb.RawRecord) error {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	prefixKey := GetMajorKeyPrefix(majorKey)
	regionName := this.GetName()

	iteratorOptions := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   100,
		Reverse:        false,
		AllVersions:    true,
	}

	iter := txn.NewIterator(iteratorOptions)
	iter.Seek(prefixKey)

	for i := 0; iter.Valid() && iter.ValidForPrefix(prefixKey); i = i + 1 {

		item := iter.Item()

		var key Key
		key.Decode(item.Key())

		msg := new(cserverpb.RawRecord)

		msg.Key = new(cserverpb.Key)
		msg.Key.RegionName = regionName
		msg.Key.MajorKey = key.MajorKey
		msg.Key.MinorKey = key.MinorKey
		msg.Key.Timestamp = key.Timestamp

		msg.Head = new(cserverpb.Head)
		msg.Head.ExpiresAt = item.ExpiresAt()
		msg.Head.Timestamp = key.Timestamp
		msg.Head.Version = item.Version()

		msg.UserMeta = []byte{item.UserMeta()}

		if item.IsDeletedOrExpired() {
			msg.Deleted = true
		} else {
			msg.Deleted = false

			data, err := item.ValueCopy(nil)
			if err != nil {
				log.Print("Error: ", err)
			} else {
				msg.Value = data
			}

		}

		outC <- msg

		iter.Next()

	}

	iter.Close()

	return nil

}




func (this *DefaultStore) Close() error {
	if this != nil && this.db != nil {
		log.Println("region closing: ", this.region.Name)
		return this.db.Close()
	}
	return nil
}

//
// Other methods
//

func (this *DefaultStore) GetEntryKey(key *cserverpb.Key) (entryKey []byte, prefixKey []byte, err error) {

	k := &Key{MajorKey: key.MajorKey, MinorKey: key.MinorKey, Timestamp: key.Timestamp}

	size := k.EncodedSize()

	if size > math.MaxUint16 {
		return nil, nil, errors.New("key is too long")
	}

	entryKey = make([]byte, size)
	PrefixLen := k.Encode(entryKey)

	return entryKey, entryKey[:PrefixLen], nil

}

func NewDefaultStore(regionDir string, region *cserverpb.Region, conf *Configuration) (context *DefaultStore, err error) {

	if _, err := os.Stat(regionDir); os.IsNotExist(err) {
		err = os.Mkdir(regionDir, 0755)
		if err != nil {
			return nil, err
		}

		str, err := new(jsonpb.Marshaler).MarshalToString(region)
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteFile(filepath.Join(regionDir, REGION_JSON), []byte(str), 0755)

		if err != nil {
			return nil, err
		}

	}

	return OpenDefaultStore(regionDir, region, conf)

}

func OpenDefaultStore(regionDir string, region *cserverpb.Region, conf *Configuration) (context *DefaultStore, err error) {

	context = &DefaultStore{regionName: region.Name, regionDir: regionDir, region: region, conf: conf}

	opts := badger.DefaultOptions
	opts.Dir = regionDir + "/key"
	opts.ValueDir = regionDir + "/value"
	context.db, err = badger.Open(opts)

	return context, err

}

func LoadBaggerDriver(regionDir string, conf *Configuration) (context *DefaultStore, err error) {

	data, err := ioutil.ReadFile(filepath.Join(regionDir, REGION_JSON))

	if err != nil {
		return nil, err
	}

	region := new(cserverpb.Region)
	err = jsonpb.UnmarshalString(string(data), region)

	if err != nil {
		return nil, err
	}

	return OpenDefaultStore(regionDir, region, conf)

}

func (this *DefaultStore) ProcessGetOperation(txn *badger.Txn, key *cserverpb.Key, operation *cserverpb.GetOperation) *cserverpb.TxOperationResult {

	lookupKey, prefixKey, err := this.GetEntryKey(key)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("format key failed: ", err))
	}

	size := 1 + int(operation.EarlyRecords)
	records := make([]*cserverpb.Record, 0, size)

	if size > 1 {

		reverseIteratorOptions := badger.IteratorOptions{
			PrefetchValues: true,
			PrefetchSize:   size,
			Reverse:        true,
			AllVersions:    false,
		}

		iter := txn.NewIterator(reverseIteratorOptions)
		iter.Seek(lookupKey)

		for i := 0; i < size && iter.Valid() && iter.ValidForPrefix(prefixKey); i = i + 1 {

			item := iter.Item()
			timestamp := GetKeyTimestamp(item.Key())

			if operation.HeadOnly {
				records = append(records, RecordHeadOf(timestamp, item))

			} else {

				data, resp := this.FetchValue(item)
				if resp != nil {
					iter.Close()
					return resp
				}

				records = append(records, RecordOf(timestamp, item, data))
			}

			iter.Next()

		}

		iter.Close()


	} else {

		item, err := txn.Get(lookupKey)
		if err != nil {
			return SuccessResultOf(records)
		}

		timestamp := key.Timestamp

		if operation.HeadOnly {
			records = append(records, RecordHeadOf(timestamp, item))

		} else {

			data, resp := this.FetchValue(item)
			if resp != nil {
				return resp
			}

			records = append(records, RecordOf(timestamp, item, data))

		}

	}

	return SuccessResultOf(records)
}

func (this *DefaultStore) ProcessRangeOperation(txn *badger.Txn, key *cserverpb.Key, operation *cserverpb.RangeOperation) *cserverpb.TxOperationResult {

	lookupKey, prefixKey, err := this.GetEntryKey(key)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("format key failed: ", err))
	}

	fmt.Print("Range LookupKey=", lookupKey, ", PrefixKey=", prefixKey, ", WithTimestamp=", key.Timestamp, ", EndMinorKey=", operation.EndMinorKey,  "\n")

	endKey := &cserverpb.Key{RegionName: key.RegionName,
							MajorKey: key.MajorKey,
							MinorKey: operation.EndMinorKey,
							Timestamp: key.Timestamp}

	_, endPrefixKey, err := this.GetEntryKey(endKey)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("format key failed: ", err))
	}

	records := make([]*cserverpb.Record, 0, 100)

	reverseIteratorOptions := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   100,
		Reverse:        false,
		AllVersions:    false,
	}

	iter := txn.NewIterator(reverseIteratorOptions)
	iter.Seek(lookupKey)

	for i := 0; iter.Valid() && !iter.ValidForPrefix(endPrefixKey); i = i + 1 {

		item := iter.Item()
		timestamp := GetKeyTimestamp(item.Key())

		if operation.HeadOnly {
			records = append(records, RecordHeadOf(timestamp, item))

		} else {

			data, resp := this.FetchValue(item)
			if resp != nil {
				iter.Close()
				return resp
			}

			records = append(records, RecordOf(timestamp, item, data))
		}

		iter.Next()

	}

	iter.Close()

	return SuccessResultOf(records)

}

func (this *DefaultStore) ProcessTouchOperation(txn *badger.Txn, key *cserverpb.Key, operation *cserverpb.TouchOperation) *cserverpb.TxOperationResult {

	lookupKey, _, err := this.GetEntryKey(key)
	entryKey := lookupKey

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("format key failed: ", err))
	}

	timestamp := key.Timestamp

	item, err := txn.Get(lookupKey)

	if err != nil {
		return SuccessNotUpdatedResult()
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return c.ErrorDriver(fmt.Sprint("touch failed: ", err))
	}

	entry := &badger.Entry{ Key: entryKey, Value:data, UserMeta: item.UserMeta()  }

	if operation.TtlSeconds > 0 {
		ttl := time.Duration(operation.TtlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("touch set entry failed: ", err))
	}

	record := RecordHeadOf(timestamp, item)

	return SuccessResultOf([]*cserverpb.Record{record})

}

func (this *DefaultStore) ProcessPutOperation(txn *badger.Txn, key *cserverpb.Key, operation *cserverpb.PutOperation) *cserverpb.TxOperationResult {

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("format key failed: ", err))
	}

	fmt.Print("Put entryKey=", entryKey, ", len=", len(entryKey), "\n")

	if operation.CompareAndSet {

		item, err := txn.Get(entryKey)

		if err != nil {
			if operation.Version != 0 {
				return SuccessNotUpdatedResult()
			}
		} else if operation.Version != item.Version() {
			return SuccessNotUpdatedResult()
		}

	}

	entry, resp := this.NewEntry(entryKey, operation.Value, operation.TtlSeconds, operation.CompressOnServer, operation.EncryptOnServer)
	if resp != nil {
		return resp
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("put failed: ", err))
	}

	return SuccessResult()

}

func (this *DefaultStore) ProcessRemoveOperation(txn *badger.Txn, key *cserverpb.Key, operation *cserverpb.RemoveOperation) *cserverpb.TxOperationResult {

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	err = txn.Delete(entryKey)

	if err != nil {
		return c.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	return SuccessResult()

}

//
// IRegionStore
//


func (this *DefaultStore) ProcessOperation(txn *badger.Txn, operation *cserverpb.TxOperation) *cserverpb.TxOperationResult {

	switch operation.Operation.(type) {

		case *cserverpb.TxOperation_Get:
			return this.ProcessGetOperation(txn, operation.Key, operation.GetGet())

		case *cserverpb.TxOperation_Range:
			return this.ProcessRangeOperation(txn, operation.Key, operation.GetRange())

		case *cserverpb.TxOperation_Touch:
			return this.ProcessTouchOperation(txn, operation.Key, operation.GetTouch())

		case *cserverpb.TxOperation_Put:
			return this.ProcessPutOperation(txn, operation.Key, operation.GetPut())

		case *cserverpb.TxOperation_Remove:
			return this.ProcessRemoveOperation(txn, operation.Key, operation.GetRemove())

	}

	return c.ErrorUnsupported("unknown operation type")
}

//
//  Value I/O
//

func (this *DefaultStore) NewEntry(entryKey, value []byte, ttlSeconds uint32, compressOnServer, encryptOnServer bool) (*badger.Entry, *cserverpb.TxOperationResult) {

	entry := &badger.Entry{ Key: entryKey, Value: value }

	if ttlSeconds > 0 {
		ttl := time.Duration(ttlSeconds) * time.Second
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	if this.conf.CompressionEnabled && compressOnServer {

		if compressedValue, err := this.conf.Compressor.Compress(entry.Value, this.conf.CompressionLevel); err == nil {
			entry.Value = compressedValue
			entry.UserMeta = SetCompressionEnabled(entry.UserMeta)
		} else {
			return nil, c.ErrorDriver(fmt.Sprint("compression failed: ", err))
		}

	}

	if this.conf.EncryptionEnabled && encryptOnServer {

		if encryptedValue, err := this.Encrypt(entry.Value); err == nil {
			entry.Value = encryptedValue
			entry.UserMeta = SetEncryptionEnabled(entry.UserMeta)
		} else {
			return nil, c.ErrorDriver(fmt.Sprint("encryption failed: ", err))
		}

	}

	return entry, nil

}


func (this *DefaultStore) FetchValue(item *badger.Item) ([]byte, *cserverpb.TxOperationResult) {

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, c.ErrorDriver(fmt.Sprint("retrieve value failed: ", err))
	}

	if this.conf.EncryptionEnabled && isEncryptionEnabled(item.UserMeta()) {

		if decrypted, err := this.Decrypt(data); err == nil {
			data = decrypted
		} else {
			return nil, c.ErrorDriver(fmt.Sprint("decryption failed: ", err))
		}

	}

	if this.conf.CompressionEnabled && isCompressionEnabled(item.UserMeta()) {

		if decompressed, err := this.conf.Compressor.Decompress(data); err == nil {
			data = decompressed
		} else {
			return nil, c.ErrorDriver(fmt.Sprint("decompress failed: ", err))
		}

	}

	return data, nil
}

//
//  Encryption
//

func (this* DefaultStore) Encrypt(plaintext []byte) ([]byte, error) {

	key, err := this.conf.SecurityContext.GetEncryptionKey(this.conf.EncryptionTopo, 0, this.conf.EncryptionKeyLen)

	if err != nil {
		return nil, err
	}

	block, err := this.conf.EncryptionCipher.Create(key)

	if err != nil {
		return nil, err
	}

	return this.conf.EncryptionMode.Encrypt(block, plaintext)

}

func (this* DefaultStore) Decrypt(ciphertext []byte) ([]byte, error) {

	key, err := this.conf.SecurityContext.GetEncryptionKey(this.conf.EncryptionTopo, 0, this.conf.EncryptionKeyLen)

	if err != nil {
		return nil, err
	}

	block, err := this.conf.EncryptionCipher.Create(key)

	if err != nil {
		return nil, err
	}

	return this.conf.EncryptionMode.Decrypt(block, ciphertext)

}
