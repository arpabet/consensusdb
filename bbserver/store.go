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

import (
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"os"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/jsonpb"
	"github.com/bigbagger/bigbagger/bbcommon"
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
	region    			*bbproto.Region
	regionDir 			string
	conf      			*Configuration

	ttl                 time.Duration    // 0 - for eternal

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

func (this *DefaultStoreTxn) ProcessOperation(op *bbproto.TxOperation) *bbproto.TxOperationResult {
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

func (this *DefaultStore) GetRegion() *bbproto.Region {
	return this.region
}

func (this *DefaultStore) NewTransaction() IRegionTnx {
	return &DefaultStoreTxn{store:this}
}

func (this *DefaultStore) GetSnapshot(majorKey []byte, outC chan<- *bbproto.RawRecord) error {

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

		msg := new(bbproto.RawRecord)

		msg.Key = new(bbproto.Key)
		msg.Key.RegionName = regionName
		msg.Key.MajorKey = key.MajorKey
		msg.Key.MinorKey = key.MinorKey
		msg.Key.Timestamp = key.Timestamp

		msg.Head = new(bbproto.Head)
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

func (this *DefaultStore) GetEntryKey(key *bbproto.Key) (entryKey []byte, prefixKey []byte, err error) {

	k := &Key{MajorKey: key.MajorKey, MinorKey: key.MinorKey, Timestamp: key.Timestamp}

	size := k.EncodedSize()

	if size > math.MaxUint16 {
		return nil, nil, errors.New("key is too long")
	}

	entryKey = make([]byte, size)
	PrefixLen := k.Encode(entryKey)

	return entryKey, entryKey[:PrefixLen], nil

}

func NewDefaultStore(regionDir string, region *bbproto.Region, conf *Configuration) (context *DefaultStore, err error) {

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

func OpenDefaultStore(regionDir string, region *bbproto.Region, conf *Configuration) (context *DefaultStore, err error) {

	context = &DefaultStore{regionName: region.Name, regionDir: regionDir, region: region, conf: conf}

	opts := badger.DefaultOptions
	opts.Dir = regionDir + "/key"
	opts.ValueDir = regionDir + "/value"
	context.db, err = badger.Open(opts)

	if len(region.Ttl) == 0 || region.Ttl == "eternal" {
		context.ttl = 0
	} else {

		context.ttl, err = bbcommon.ParseTtlExpr(region.Ttl)

		if err != nil {
			return nil, err
		}

	}

	return context, err

}

func LoadBaggerDriver(regionDir string, conf *Configuration) (context *DefaultStore, err error) {

	data, err := ioutil.ReadFile(filepath.Join(regionDir, REGION_JSON))

	if err != nil {
		return nil, err
	}

	region := new(bbproto.Region)
	err = jsonpb.UnmarshalString(string(data), region)

	if err != nil {
		return nil, err
	}

	return OpenDefaultStore(regionDir, region, conf)

}

func (this *DefaultStore) ProcessGetOperation(txn *badger.Txn, key *bbproto.Key, operation *bbproto.GetOperation) *bbproto.TxOperationResult {

	lookupKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	timestamp := key.Timestamp

	item, err := txn.Get(lookupKey)
	if err != nil {
		return SuccessGetNotFoundResult()
	}

	if operation.HeadOnly {

		return SuccessHeadResult(timestamp, item)

	} else {

		data, resp := this.FetchValue(item)
		if resp != nil {
			return resp
		}

		return SuccessGetResult(timestamp, item, data)

	}


}

func (this *DefaultStore) ProcessRangeOperation(txn *badger.Txn, key *bbproto.Key, operation *bbproto.RangeOperation) *bbproto.TxOperationResult {

	lookupKey, prefixKey, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	fmt.Print("Range LookupKey=", lookupKey, ", PrefixKey=", prefixKey, ", WithTimestamp=", key.Timestamp, ", NumRecords=", operation.NumRecords,  "\n")

	size := int(operation.NumRecords)
	records := make([]*bbproto.Record, 0, size)

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
			records = append(records, &bbproto.Record{Head: HeadOf(timestamp, item)})

		} else {

			data, resp := this.FetchValue(item)
			if resp != nil {
				return resp
			}

			records = append(records, RecordOf(timestamp, item, data))
		}

		iter.Next()

	}

	iter.Close()

	return SuccessRangeResult(records)

}

func (this *DefaultStore) ProcessTouchOperation(txn *badger.Txn, key *bbproto.Key, operation *bbproto.TouchOperation) *bbproto.TxOperationResult {

	lookupKey, _, err := this.GetEntryKey(key)
	entryKey := lookupKey

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	timestamp := key.Timestamp

	item, err := txn.Get(lookupKey)

	if err != nil {
		return SuccessTouchNotFoundResult()
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch failed: ", err))
	}

	ttl := this.ttl
	if operation.OverrideTtl {
		ttl = time.Duration(operation.TtlSeconds) * time.Second
	}

	entry := &badger.Entry{ Key: entryKey, Value:data, UserMeta: item.UserMeta()  }

	if ttl > 0 {
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch set entry failed: ", err))
	}

	return SuccessTouchResult(timestamp, item, entry.ExpiresAt)

}

func (this *DefaultStore) ProcessPutOperation(txn *badger.Txn, key *bbproto.Key, operation *bbproto.PutOperation) *bbproto.TxOperationResult {

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	fmt.Print("Put entryKey=", entryKey, ", len=", len(entryKey), "\n")

	if operation.CompareAndSet {

		item, err := txn.Get(entryKey)

		if err != nil {
			if operation.Version != 0 {
				return SuccessPutResult(false)
			}
		} else if operation.Version != item.Version() {
			return SuccessPutResult(false)
		}

	}

	ttl := this.ttl
	if operation.OverrideTtl {
		ttl = time.Duration(operation.TtlSeconds) * time.Second
	}

	entry, resp := this.NewEntry(entryKey, operation.Value, ttl, operation.CompressOnServer, operation.EncryptOnServer)
	if resp != nil {
		return resp
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put failed: ", err))
	}

	return SuccessPutResult(true)

}

func (this *DefaultStore) ProcessRemoveOperation(txn *badger.Txn, key *bbproto.Key, operation *bbproto.RemoveOperation) *bbproto.TxOperationResult {

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	err = txn.Delete(entryKey)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	return SuccessRemoveResult(true)

}

//
// IRegionStore
//


func (this *DefaultStore) ProcessOperation(txn *badger.Txn, operation *bbproto.TxOperation) *bbproto.TxOperationResult {

	switch operation.Operation.(type) {

		case *bbproto.TxOperation_Get:
			return this.ProcessGetOperation(txn, operation.Key, operation.GetGet())

		case *bbproto.TxOperation_Range:
			return this.ProcessRangeOperation(txn, operation.Key, operation.GetRange())

		case *bbproto.TxOperation_Touch:
			return this.ProcessTouchOperation(txn, operation.Key, operation.GetTouch())

		case *bbproto.TxOperation_Put:
			return this.ProcessPutOperation(txn, operation.Key, operation.GetPut())

		case *bbproto.TxOperation_Remove:
			return this.ProcessRemoveOperation(txn, operation.Key, operation.GetRemove())

	}

	return bbcommon.ErrorUnsupported("unknown operation type")
}

//
//  Value I/O
//

func (this *DefaultStore) NewEntry(entryKey, value []byte, ttl time.Duration, compressOnServer, encryptOnServer bool) (*badger.Entry, *bbproto.TxOperationResult) {

	entry := &badger.Entry{ Key: entryKey, Value: value }

	if ttl > 0 {
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	if this.conf.CompressionEnabled && compressOnServer {

		if compressedValue, err := this.conf.Compressor.Compress(entry.Value, this.conf.CompressionLevel); err == nil {
			entry.Value = compressedValue
			entry.UserMeta = SetCompressionEnabled(entry.UserMeta)
		} else {
			return nil, bbcommon.ErrorDriver(fmt.Sprint("compression failed: ", err))
		}

	}

	if this.conf.EncryptionEnabled && encryptOnServer {

		if encryptedValue, err := this.Encrypt(entry.Value); err == nil {
			entry.Value = encryptedValue
			entry.UserMeta = SetEncryptionEnabled(entry.UserMeta)
		} else {
			return nil, bbcommon.ErrorDriver(fmt.Sprint("encryption failed: ", err))
		}

	}

	return entry, nil

}


func (this *DefaultStore) FetchValue(item *badger.Item) ([]byte, *bbproto.TxOperationResult) {

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, bbcommon.ErrorDriver(fmt.Sprint("retrieve value failed: ", err))
	}

	if this.conf.EncryptionEnabled && isEncryptionEnabled(item.UserMeta()) {

		if decrypted, err := this.Decrypt(data); err == nil {
			data = decrypted
		} else {
			return nil, bbcommon.ErrorDriver(fmt.Sprint("decryption failed: ", err))
		}

	}

	if this.conf.CompressionEnabled && isCompressionEnabled(item.UserMeta()) {

		if decompressed, err := this.conf.Compressor.Decompress(data); err == nil {
			data = decompressed
		} else {
			return nil, bbcommon.ErrorDriver(fmt.Sprint("decompress failed: ", err))
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
