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

import (
	"github.com/dgraph-io/badger"
	"bigbagger/proto/bbproto"
	"os"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/jsonpb"
	"bigbagger/bbcommon"
	"fmt"
	"time"
	"log"
	"github.com/pkg/errors"
)

const (
	DATASET_JSON = "dataset.json"
)

type DatasetContext struct {
	db                     *badger.DB
	dataset                *bbproto.Dataset
	security               ISecurity

	compressionEnabled     bool
	compressThreshold      int
	compressor             ICompressor
	compressionLevel       bbproto.CompressionLevel

	encryptionEnabled      bool
	encryptionCipher       ICipher
	encryptionMode         IBlockMode
	encryptionTopo         string
	encryptionBlockSize    int

}

func (this *DatasetContext) GetName() string {
	return this.dataset.Name
}

func (this *DatasetContext) Close() error {
	if this != nil && this.db != nil {
		log.Println("dataset closing: ", this.dataset.Name)
		return this.db.Close()
	}
	return nil
}

func NewDataset(dbDir string, dataset *bbproto.Dataset, security ISecurity) (context *DatasetContext, err error) {

	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err = os.Mkdir(dbDir, 0755)
		if err != nil {
			return nil, err
		}

		str, err := new(jsonpb.Marshaler).MarshalToString(dataset)
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteFile(filepath.Join(dbDir, DATASET_JSON), []byte(str), 0755)

		if err != nil {
			return nil, err
		}

	}

	return OpenDataset(dbDir, dataset, security)

}

func OpenDataset(dbDir string, dataset *bbproto.Dataset, security ISecurity) (context *DatasetContext, err error) {

	context = new(DatasetContext)
	context.dataset = dataset
	context.security = security

	opts := badger.DefaultOptions
	opts.Dir = dbDir + "/key"
	opts.ValueDir = dbDir + "/value"
	context.db, err = badger.Open(opts)

	if dataset.Compression != nil && dataset.Compression.Compressor != bbproto.Compressor_COMPRESS_NO {

		compressor, ok := KnownCompressors[dataset.Compression.Compressor]

		if !ok {
			return nil, errors.New("compression algorithm not found: " + dataset.Compression.Compressor.String())
		}

		context.compressionEnabled = true
		context.compressThreshold = int(dataset.Compression.Threshold)
		context.compressor = compressor
		context.compressionLevel = dataset.Compression.Level

	}

	if dataset.Encryption != nil && dataset.Encryption.Cipher != bbproto.Cipher_CIPHER_NO {

		if dataset.Encryption.Mode == bbproto.BlockMode_MODE_NO {
			return nil, errors.New("empty block mode")
		}

		if dataset.Encryption.Size == bbproto.BlockSize_BLOCK_NO {
			return nil, errors.New("empty block size")
		}

		cipher, ok := KnownCiphers[dataset.Encryption.Cipher]

		if !ok {
			return nil, errors.New("cipher not found " + dataset.Encryption.Cipher.String())
		}

		mode, ok := KnownBlockModes[dataset.Encryption.Mode]

		if !ok {
			return nil, errors.New("block mode not found " + dataset.Encryption.Mode.String())
		}

		context.encryptionEnabled = true
		context.encryptionCipher = cipher
		context.encryptionMode = mode
		context.encryptionTopo = dataset.Encryption.Topo
		context.encryptionBlockSize = int(dataset.Encryption.Size)


	}

	return context, err

}

func LoadDataset(dbDir string, security ISecurity) (context *DatasetContext, err error) {

	data, err := ioutil.ReadFile(filepath.Join(dbDir, DATASET_JSON))

	if err != nil {
		return nil, err
	}

	dataset := new(bbproto.Dataset)
	err = jsonpb.UnmarshalString(string(data), dataset)

	if err != nil {
		return nil, err
	}

	return OpenDataset(dbDir, dataset, security)

}

func (this *DatasetContext) ProcessHeadOperation(key *bbproto.Key, operation *bbproto.HeadOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)

	if err != nil {
		return SuccessHeadNotFoundResult()
	}

	return SuccessHeadResult(key.Timestamp, item)

}

func (this *DatasetContext) ProcessGetOperation(key *bbproto.Key, operation *bbproto.GetOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)
	if err != nil {
		return SuccessGetNotFoundResult()
	}

	data, err := item.Value()
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get failed: ", err))
	}

	if this.encryptionEnabled && isEncryptionEnabled(item.UserMeta()) {

		if decrypted, err := this.Decrypt(data); err == nil {
			data = decrypted
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("decryption failed: ", err))
		}

	}

	if this.compressionEnabled && isCompressionEnabled(item.UserMeta()) {

		if decompressed, err := this.compressor.Decompress(data); err == nil {
			data = decompressed
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("decompress failed: ", err))
		}

	}

	return SuccessGetResult(key.Timestamp, data, item)

}

func (this *DatasetContext) ProcessTouchOperation(key *bbproto.Key, operation *bbproto.TouchOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)

	if err != nil {
		return SuccessTouchNotFoundResult()
	}

	data, err := item.Value()
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch failed: ", err))
	}

	ttlSeconds := this.dataset.TtlSeconds
	if operation.OverrideTtl {
		ttlSeconds = operation.TtlSeconds
	}

	entry := &badger.Entry{ Key: key.RecordKey, Value:data, UserMeta: item.UserMeta()  }

	if ttlSeconds > 0 {
		expire := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	err = txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch set entry failed: ", err))
	}

	err = txn.Commit(nil)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("touch commit failed: ", err))
	}

	return SuccessTouchResult(key.Timestamp, item, entry.ExpiresAt)

}

func (this *DatasetContext) ProcessPutOperation(key *bbproto.Key, operation *bbproto.PutOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	if operation.CompareAndSet {

		item, err := txn.Get(key.RecordKey)

		if err != nil {
			if operation.Version != 0 {
				return SuccessPutResult(false)
			}
		} else if operation.Version != item.Version() {
			return SuccessPutResult(false)
		}

	}

	ttlSeconds := this.dataset.TtlSeconds
	if operation.OverrideTtl {
		ttlSeconds = operation.TtlSeconds
	}

	entry := &badger.Entry{ Key: key.RecordKey, Value: operation.Value  }

	if ttlSeconds > 0 {
		expire := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
		entry.ExpiresAt = uint64(expire)
	}

	if this.compressionEnabled && len(operation.Value) >= this.compressThreshold {

		if compressedValue, err := this.compressor.Compress(entry.Value, this.compressionLevel); err == nil {
			entry.Value = compressedValue
			entry.UserMeta = SetCompressionEnabled(entry.UserMeta)
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("compression failed: ", err))
		}

	}

	if this.encryptionEnabled {

		if encryptedValue, err := this.Encrypt(entry.Value); err == nil {
			entry.Value = encryptedValue
			entry.UserMeta = SetEncryptionEnabled(entry.UserMeta)
		} else {
			return bbcommon.ErrorDriver(fmt.Sprint("encryption failed: ", err))
		}

	}

	err := txn.SetEntry(entry)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put failed: ", err))
	}

	err = txn.Commit(nil)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put commit failed: ", err))
	}

	return SuccessPutResult(true)

}

func (this *DatasetContext) ProcessRemoveOperation(key *bbproto.Key, operation *bbproto.RemoveOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	err := txn.Delete(key.RecordKey)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	err = txn.Commit(nil)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove commit failed: ", err))
	}

	return SuccessRemoveResult(true)

}


func (this *DatasetContext) ProcessOperation(operation *bbproto.RecordOperation) *bbproto.RecordResult {

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

func (this* DatasetContext) Encrypt(plaintext []byte) ([]byte, error) {

	key, err := this.security.GetEncryptionKey(this.encryptionTopo, 0, this.encryptionBlockSize)

	if err != nil {
		return nil, err
	}

	block, err := this.encryptionCipher.Create(key)

	if err != nil {
		return nil, err
	}

	return this.encryptionMode.Encrypt(block, plaintext)

}

func (this* DatasetContext) Decrypt(ciphertext []byte) ([]byte, error) {

	key, err := this.security.GetEncryptionKey(this.encryptionTopo, 0, this.encryptionBlockSize)

	if err != nil {
		return nil, err
	}

	block, err := this.encryptionCipher.Create(key)

	if err != nil {
		return nil, err
	}

	return this.encryptionMode.Decrypt(block, ciphertext)

}
