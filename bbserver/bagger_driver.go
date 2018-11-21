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
	"encoding/binary"
	"math"
	"github.com/bigbagger/bagger"
)

var ReverseIteratorOptions = bagger.IteratorOptions{
	PrefetchValues: true,
	PrefetchSize:   1,
	Reverse:        true,
	AllVersions:    false,
}


type BaggerDriver struct {
	db                     *bagger.DB
	table                  *bbproto.Table
	dbDir                  string
	conf                   *Configuration

	ttl                    time.Duration    // 0 - for eternal

	pitEnabled             bool
	pitPrimaryTimestamp    bool
	pitConflate            bool

}

//
// IDriver
//

func (this *BaggerDriver) GetTable() *bbproto.Table {
	return this.table
}

//
// IDriver
//

func (this *BaggerDriver) Close() error {
	if this != nil && this.db != nil {
		log.Println("table closing: ", this.table.Name)
		return this.db.Close()
	}
	return nil
}

func (this *BaggerDriver) GetEntryKey(key *bbproto.Key) (fullKey []byte, prefixKey []byte, err error) {

	if this.pitEnabled {

		recordKey := key.RecordKey
		recordKeyLen := len(recordKey)
		if recordKeyLen > 255 {
			return nil, nil, errors.New("recordKey is too long")
		}

		fullKey = make([]byte, 1+recordKeyLen+8)
		fullKey[0] = byte(recordKeyLen)
		copy(fullKey[1:], recordKey)
		binary.BigEndian.PutUint64(fullKey[recordKeyLen+1:], key.Timestamp)
		//binary.BigEndian.PutUint64(fullKey[recordKeyLen+1:], math.MaxUint64 - key.Timestamp)

		return fullKey, fullKey[:recordKeyLen+1], nil

	} else {
		return key.RecordKey, key.RecordKey, nil
	}

}

func (this *BaggerDriver) GetTimestamp(fullKey []byte) uint64 {

	if len(fullKey) <= 9 {
		return 0
	}
	return binary.BigEndian.Uint64(fullKey[len(fullKey)-8:])
	//return math.MaxUint64 - binary.BigEndian.Uint64(fullKey[len(fullKey)-8:])

}

func (this *BaggerDriver) GetEntryKeyPrefix(key *bbproto.Key) ([]byte) {

	if this.pitEnabled {

		recordKey := key.RecordKey
		recordKeyLen := len(recordKey)

		out := make([]byte, recordKeyLen+1)
		out[0] = byte(recordKeyLen)
		copy(out[1:], recordKey)

		return out

	} else {
		return key.RecordKey
	}

}

func NewBaggerDriver(dbDir string, table *bbproto.Table, conf *Configuration) (context *BaggerDriver, err error) {

	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err = os.Mkdir(dbDir, 0755)
		if err != nil {
			return nil, err
		}

		str, err := new(jsonpb.Marshaler).MarshalToString(table)
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteFile(filepath.Join(dbDir, TABLE_JSON), []byte(str), 0755)

		if err != nil {
			return nil, err
		}

	}

	return OpenBaggerDriver(dbDir, table, conf)

}

func OpenBaggerDriver(dbDir string, table *bbproto.Table, conf *Configuration) (context *BaggerDriver, err error) {

	context = &BaggerDriver{dbDir: dbDir, table: table, conf: conf}

	opts := bagger.DefaultOptions
	opts.Dir = dbDir + "/key"
	opts.ValueDir = dbDir + "/value"
	context.db, err = bagger.Open(opts)

	if table.Pit != nil {
		context.pitEnabled = true
		context.pitPrimaryTimestamp = table.Pit.PrimaryTimestamp
		context.pitConflate = table.Pit.Conflation
	}

	if len(table.Ttl) == 0 || table.Ttl == "eternal" {
		context.ttl = 0
	} else {

		context.ttl, err = bbcommon.ParseTtlExpr(table.Ttl)

		if err != nil {
			return nil, err
		}

	}

	return context, err

}

func LoadBaggerDriver(dbDir string, conf *Configuration) (context *BaggerDriver, err error) {

	data, err := ioutil.ReadFile(filepath.Join(dbDir, TABLE_JSON))

	if err != nil {
		return nil, err
	}

	table := new(bbproto.Table)
	err = jsonpb.UnmarshalString(string(data), table)

	if err != nil {
		return nil, err
	}

	return OpenBaggerDriver(dbDir, table, conf)

}

func (this *BaggerDriver) ProcessHeadOperation(key *bbproto.Key, operation *bbproto.HeadOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	if key.Timestamp == 0 {
		key.Timestamp = math.MaxUint64
	}

	entryKey, prefixKey, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	fmt.Print("Head EntryKey=", entryKey, ", PrefixKey=", prefixKey, "\n")

	timestamp := key.Timestamp
	var item *bagger.Item

	if this.pitEnabled {

		iter := txn.NewIterator(ReverseIteratorOptions)

		iter.Seek(entryKey)

		if iter.Valid() && iter.ValidForPrefix(prefixKey) {

			item = iter.Item()
			timestamp = this.GetTimestamp(item.Key())

		} else {
			iter.Close()
			return SuccessHeadNotFoundResult()
		}

		iter.Close()

	} else {
		item, err = txn.Get(entryKey)

		if err != nil {
			return SuccessHeadNotFoundResult()
		}
	}

	fmt.Print("item=", item, "\n")

	if err != nil {
		return SuccessHeadNotFoundResult()
	}

	return SuccessHeadResult(timestamp, item)

}

func (this *BaggerDriver) ProcessGetOperation(key *bbproto.Key, operation *bbproto.GetOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	item, err := txn.Get(entryKey)
	if err != nil {
		return SuccessGetNotFoundResult()
	}

	data, err := item.ValueCopy(nil)
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get failed: ", err))
	}

	//dataCopied := false

	if this.conf.EncryptionEnabled && isEncryptionEnabled(item.UserMeta()) {

		if decrypted, err := this.Decrypt(data); err == nil {
			data = decrypted
			//dataCopied = true
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("decryption failed: ", err))
		}

	}

	if this.conf.CompressionEnabled && isCompressionEnabled(item.UserMeta()) {

		if decompressed, err := this.conf.Compressor.Decompress(data); err == nil {
			data = decompressed
			//dataCopied = true
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("decompress failed: ", err))
		}

	}

	// copy data because outside of the transaction they will be destroyed
	//if !dataCopied {
	//	data = bbcommon.CopyOf(data)
	//}

	return SuccessGetResult(key.Timestamp, data, item)

}

func (this *BaggerDriver) ProcessTouchOperation(key *bbproto.Key, operation *bbproto.TouchOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	item, err := txn.Get(entryKey)

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

	entry := &bagger.Entry{ Key: entryKey, Value:data, UserMeta: item.UserMeta()  }

	if ttl > 0 {
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch set entry failed: ", err))
	}

	err = txn.Commit()

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch commit failed: ", err))
	}

	return SuccessTouchResult(key.Timestamp, item, entry.ExpiresAt)

}

func (this *BaggerDriver) ProcessPutOperation(key *bbproto.Key, operation *bbproto.PutOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	fmt.Print("Put EntryKey=", entryKey, "\n")

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

	entry := &bagger.Entry{ Key: entryKey, Value: operation.Value  }

	if ttl > 0 {
		expire := time.Now().Add(ttl).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	if this.conf.CompressionEnabled && operation.CompressOnServer && len(operation.Value) >= this.conf.CompressionThreshold {

		if compressedValue, err := this.conf.Compressor.Compress(entry.Value, this.conf.CompressionLevel); err == nil {
			entry.Value = compressedValue
			entry.UserMeta = SetCompressionEnabled(entry.UserMeta)
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("compression failed: ", err))
		}

	}

	if this.conf.EncryptionEnabled && operation.EncryptOnServer {

		if encryptedValue, err := this.Encrypt(entry.Value); err == nil {
			entry.Value = encryptedValue
			entry.UserMeta = SetEncryptionEnabled(entry.UserMeta)
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("encryption failed: ", err))
		}

	}

	err = txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put failed: ", err))
	}

	err = txn.Commit()

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put commit failed: ", err))
	}

	return SuccessPutResult(true)

}

func (this *BaggerDriver) ProcessRemoveOperation(key *bbproto.Key, operation *bbproto.RemoveOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	entryKey, _, err := this.GetEntryKey(key)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get entry key failed: ", err))
	}

	err = txn.Delete(entryKey)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	err = txn.Commit()

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove commit failed: ", err))
	}

	return SuccessRemoveResult(true)

}

//
// IDriver
//


func (this *BaggerDriver) ProcessOperation(operation *bbproto.RecordOperation) *bbproto.RecordResult {

	switch operation.Operation.(type) {

		case *bbproto.RecordOperation_Head:
			return this.ProcessHeadOperation(operation.Key, operation.GetHead())

		case *bbproto.RecordOperation_Get:
			return this.ProcessGetOperation(operation.Key, operation.GetGet())

		case *bbproto.RecordOperation_Touch:
			return this.ProcessTouchOperation(operation.Key, operation.GetTouch())

		case *bbproto.RecordOperation_Put:
			return this.ProcessPutOperation(operation.Key, operation.GetPut())

		case *bbproto.RecordOperation_Remove:
			return this.ProcessRemoveOperation(operation.Key, operation.GetRemove())

	}

	return bbcommon.ErrorUnsupported("unknown operation type")
}

func (this* BaggerDriver) Encrypt(plaintext []byte) ([]byte, error) {

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

func (this* BaggerDriver) Decrypt(ciphertext []byte) ([]byte, error) {

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
