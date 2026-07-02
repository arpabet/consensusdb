/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"context"
	"encoding/binary"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/store"
	badger "github.com/dgraph-io/badger/v4"
	"golang.org/x/xerrors"
	"go.uber.org/zap"
	"io"
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

	// Touch resets TTL on an existing record. commitVersion, when non-zero, is
	// the raft log index the caller (FSM) assigns as the new envelope version so
	// every replica lands the same value; 0 means assign locally (raft-off).
	Touch(recordRequest *pb.RecordRequest, commitVersion uint64) (*pb.Status, error);

	// Put writes a record. commitVersion, when non-zero, is the raft log index
	// stamped into the value envelope as the replica-independent version; 0 means
	// assign locally (envelope version = previous+1) for the single-node path.
	Put(recordRequest *pb.RecordRequest, commitVersion uint64) (*pb.Status, error);

	Remove(keyRequest *pb.KeyRequest) (*pb.Status, error);

	// Increment atomically adds delta to the int64 counter at key (starting from
	// initial when absent) and returns the previous value plus the new envelope
	// version. commitVersion is the raft log index (0 = assign locally).
	Increment(req *pb.IncrementRequest, commitVersion uint64) (*pb.IncrementResponse, error);

	// SetBatch writes every record in one badger transaction (all-or-nothing).
	// commitVersion is the raft log index stamped into each entry's envelope
	// (0 = assign locally per key).
	SetBatch(req *pb.BatchRequest, commitVersion uint64) (*pb.Status, error);

	// WatchRaw streams change events for keys under the (encoded) prefix from this
	// node's local hub until cb returns false or ctx is done. A nil/empty prefix
	// watches every key. Delivery is best-effort (see store.WatchHub).
	WatchRaw(ctx context.Context, prefix []byte, cb func(*store.WatchEvent) bool) error;

	// Backup writes a self-describing snapshot of the whole store to w and
	// returns the last applied version. Used by the raft FSM snapshot.
	Backup(w io.Writer) (uint64, error)

	// Load restores the store contents from a stream produced by Backup.
	// Used by the raft FSM restore.
	Load(r io.Reader) error

	Close() error

}

type KeyValueStorageCtx struct {

	db        			*badger.DB
	conf      			*Configuration
	log                 *zap.Logger
	hub                 *store.WatchHub

}

func OpenKeyValueStorage(conf *Configuration, log *zap.Logger) (context *KeyValueStorageCtx, err error) {

	context = &KeyValueStorageCtx{conf: conf, log: log, hub: store.NewWatchHub()}

	opts := badger.DefaultOptions(conf.DataDir)
	opts.Dir = conf.KeyDir
	opts.ValueDir = conf.ValueDir
	opts.CompactL0OnClose = true
	// badger v4 removed the table / value-log loading-mode toggles
	// (memory-map vs file-io); memory management is now automatic, so
	// conf.FileIO no longer maps onto a badger option.
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

// loadMaxPendingWrites bounds badger.Load concurrency during snapshot restore.
const loadMaxPendingWrites = 256

func (this *KeyValueStorageCtx) Backup(w io.Writer) (uint64, error) {
	return this.db.Backup(w, 0)
}

func (this *KeyValueStorageCtx) Load(r io.Reader) error {
	return this.db.Load(r, loadMaxPendingWrites)
}

// notify fans a committed mutation out to local watchers. It is called by the
// storage write methods, which on the replicated path are driven by the raft FSM
// apply on every node — so watchers on any node observe changes committed through
// raft (cross-node, best-effort). key is the encoded entry key. value is nil for
// deletes. Copies are made because the caller's buffers may be reused.
func (this *KeyValueStorageCtx) notify(key, value []byte, eventType store.WatchEventType, version int64) {
	if this.hub == nil {
		return
	}
	k := make([]byte, len(key))
	copy(k, key)
	var val []byte
	if value != nil {
		val = make([]byte, len(value))
		copy(val, value)
	}
	this.hub.Notify(&store.WatchEvent{Key: k, Value: val, Type: eventType, Version: version})
}

func (this *KeyValueStorageCtx) WatchRaw(ctx context.Context, prefix []byte, cb func(*store.WatchEvent) bool) error {
	return this.hub.Watch(ctx, prefix, cb)
}

/*
StorageBean is the glue lifecycle wrapper around the badger-backed
KeyValueStorage. It opens the database on PostConstruct and closes it on Destroy.
It implements KeyValueStorage by delegating to the opened context, so it can be
injected wherever a KeyValueStorage is required (e.g. the gRPC service).
*/
type StorageBean struct {
	Conf    *Configuration `inject:""`
	Log     *zap.Logger    `inject:""`
	storage *KeyValueStorageCtx
}

func (t *StorageBean) PostConstruct() error {
	storage, err := OpenKeyValueStorage(t.Conf, t.Log)
	if err != nil {
		return err
	}
	t.storage = storage
	return nil
}

func (t *StorageBean) Destroy() error {
	return t.Close()
}

func (t *StorageBean) Get(r *pb.KeyRequest) (*pb.Record, error)       { return t.storage.Get(r) }
func (t *StorageBean) GetRecent(r *pb.KeyRequest) (*pb.Record, error) { return t.storage.GetRecent(r) }
func (t *StorageBean) GetRange(r *pb.RangeRequest) (*pb.Block, error) { return t.storage.GetRange(r) }
func (t *StorageBean) GetArea(r *pb.KeyRequest, lastField Field, sender BlockSender) error {
	return t.storage.GetArea(r, lastField, sender)
}
func (t *StorageBean) Scan(r *pb.ScanRequest, sender BlockSender) error { return t.storage.Scan(r, sender) }
func (t *StorageBean) Touch(r *pb.RecordRequest, commitVersion uint64) (*pb.Status, error) {
	return t.storage.Touch(r, commitVersion)
}
func (t *StorageBean) Put(r *pb.RecordRequest, commitVersion uint64) (*pb.Status, error) {
	return t.storage.Put(r, commitVersion)
}
func (t *StorageBean) Remove(r *pb.KeyRequest) (*pb.Status, error) { return t.storage.Remove(r) }
func (t *StorageBean) Increment(r *pb.IncrementRequest, commitVersion uint64) (*pb.IncrementResponse, error) {
	return t.storage.Increment(r, commitVersion)
}
func (t *StorageBean) SetBatch(r *pb.BatchRequest, commitVersion uint64) (*pb.Status, error) {
	return t.storage.SetBatch(r, commitVersion)
}
func (t *StorageBean) WatchRaw(ctx context.Context, prefix []byte, cb func(*store.WatchEvent) bool) error {
	return t.storage.WatchRaw(ctx, prefix, cb)
}

func (t *StorageBean) Backup(w io.Writer) (uint64, error) { return t.storage.Backup(w) }
func (t *StorageBean) Load(r io.Reader) error             { return t.storage.Load(r) }

func (t *StorageBean) Close() error {
	if t.storage != nil {
		return t.storage.Close()
	}
	return nil
}

// FetchRecord materializes a stored badger item into a pb.Record, unwrapping the
// store value envelope (version | expiresAt | value). The version and expiry are
// read from the envelope, NOT from badger's node-local item.Version() — that is
// what keeps CAS and the reported version identical across raft replicas.
//
// Because the version now lives inside the value, headOnly still copies the value
// to read the 17-byte envelope header; it just omits the payload from the reply.
func FetchRecord(key *pb.Key, item *badger.Item, headOnly bool) (*pb.Record, error) {

	data, err := item.ValueCopy(nil)
	if err != nil {
		return nil, xerrors.Errorf("fetch value failed: %v", err)
	}

	version, expiresAt, value, _ := store.DecodeEnvelope(data)

	// Lazy read-time expiry: an expired entry is hidden (reported as not found),
	// exactly like store's envelope providers. It stays physically present until a
	// deterministic sweep removes it — reads must not delete, or replicas would
	// diverge. Existence checks for CAS/Increment use raw txn.Get, not this path,
	// so an expired-but-unswept key still blocks putIfAbsent deterministically.
	if store.IsExpired(expiresAt) {
		return RecordNotFound(key), nil
	}

	diskSize := item.EstimatedSize()
	meta := int32(item.UserMeta())

	if headOnly {
		return RecordHead(key, uint64(version), uint64(expiresAt), diskSize, meta), nil
	}

	return RecordValue(key, uint64(version), uint64(expiresAt), diskSize, meta, value), nil

}

// resolveExpiry returns the absolute expiry (unix seconds) to store. It prefers an
// expiresAt already computed on the leader (so every replica stores the same
// value); otherwise it derives one from the relative ttlSeconds. This fallback
// keeps direct storage callers (and single-node tests) working, while the service
// layer sets expiresAt up front on the replicated path.
func resolveExpiry(ttlSeconds int64, expiresAt int64) int64 {
	if expiresAt != 0 {
		return expiresAt
	}
	if ttlSeconds > 0 {
		return store.ExpiryFromTtl(int(ttlSeconds))
	}
	return 0
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

		if record.Head == nil {
			// expired (hidden on read) — skip without consuming a slot
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

		if record.Head == nil {
			// expired (hidden on read) — skip
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

		if record.Head == nil {
			// expired (hidden on read) — skip
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

func (this *KeyValueStorageCtx) Touch(recordRequest *pb.RecordRequest, commitVersion uint64) (*pb.Status, error) {

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
		return nil, xerrors.Errorf("fetch in touch error: %v", err)
	}

	// Preserve the stored value, re-stamp expiry and a fresh version. A touch is
	// a replicated write, so it advances the version deterministically too.
	oldVersion, _, value, _ := store.DecodeEnvelope(data)

	version := commitVersion
	if version == 0 {
		version = uint64(oldVersion) + 1
	}

	expiresAt := resolveExpiry(recordRequest.TtlSeconds, recordRequest.ExpiresAt)

	envelope := store.EncodeEnvelope(int64(version), expiresAt, value)
	entry := &badger.Entry{ Key: entryKey, Value: envelope, UserMeta: item.UserMeta() }

	if err := txn.SetEntry(entry); err != nil {
		return nil, xerrors.Errorf("update entry error: %v", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, xerrors.Errorf("commit touch error: %v", err)
	}

	this.notify(entryKey, value, store.WatchSet, int64(version))

	return &pb.Status{ Updated: true}, nil

}

func (this *KeyValueStorageCtx) Put(recordRequest *pb.RecordRequest, commitVersion uint64) (*pb.Status, error) {

	if recordRequest.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(recordRequest.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	// Read the current envelope version (if any). CAS compares against this
	// envelope version, never badger's item.Version() (which is node-local).
	var oldVersion uint64
	exists := false
	if item, err := txn.Get(entryKey); err == nil {
		exists = true
		data, err := item.ValueCopy(nil)
		if err != nil {
			return nil, xerrors.Errorf("read current value error: %v", err)
		}
		v, _, _, _ := store.DecodeEnvelope(data)
		oldVersion = uint64(v)
	} else if err != badger.ErrKeyNotFound {
		return nil, err
	}

	if recordRequest.CompareAndSet {
		if !exists {
			if recordRequest.Version != 0 { // expected an existing version
				return &pb.Status{ Updated: false}, nil
			}
			// putIfAbsent (version 0 == absent)
		} else if recordRequest.Version != oldVersion {
			// wrong version CAS
			return &pb.Status{ Updated: false}, nil
		}
	}

	// commitVersion is the raft log index on the replicated path; on the
	// single-node / raft-off path it is 0 and we fall back to a per-key counter.
	version := commitVersion
	if version == 0 {
		version = oldVersion + 1
	}

	expiresAt := resolveExpiry(recordRequest.TtlSeconds, recordRequest.ExpiresAt)

	// The envelope carries expiry; badger's native ExpiresAt is deliberately NOT
	// set — its node-local wall-clock drop would hide keys at different times per
	// replica and break Apply's existence checks. Expiry is enforced on read and
	// reclaimed by a deterministic sweep.
	envelope := store.EncodeEnvelope(int64(version), expiresAt, recordRequest.Value)
	entry := &badger.Entry{ Key: entryKey, Value: envelope, UserMeta: byte(recordRequest.Metadata) }

	if err := txn.SetEntry(entry); err != nil {
		return nil, xerrors.Errorf("update entry error: %v", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, xerrors.Errorf("commit put error: %v", err)
	}

	this.notify(entryKey, recordRequest.Value, store.WatchSet, int64(version))

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
		return nil, xerrors.Errorf("delete entry error: %v", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, xerrors.Errorf("commit remove error: %v", err)
	}

	this.notify(entryKey, nil, store.WatchDelete, 0)

	return &pb.Status{ Updated: true}, nil

}

// counterFromEnvelope decodes the 8-byte big-endian counter payload from a stored
// envelope value, returning the current version and the counter (fallback when the
// payload is shorter than 8 bytes, e.g. a freshly created key).
func counterFromEnvelope(data []byte, fallback int64) (version uint64, counter int64) {
	v, _, val, _ := store.DecodeEnvelope(data)
	counter = fallback
	if len(val) >= 8 {
		counter = int64(binary.BigEndian.Uint64(val))
	}
	return uint64(v), counter
}

func (this *KeyValueStorageCtx) Increment(req *pb.IncrementRequest, commitVersion uint64) (*pb.IncrementResponse, error) {

	if req.Key == nil {
		return nil, ErrorEmptyKey
	}

	entryKey, _ := EncodeKey(req.Key)

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	// Read the current counter and envelope version. Absent key starts at Initial.
	var oldVersion uint64
	counter := req.Initial
	if item, err := txn.Get(entryKey); err == nil {
		data, err := item.ValueCopy(nil)
		if err != nil {
			return nil, xerrors.Errorf("read counter error: %v", err)
		}
		oldVersion, counter = counterFromEnvelope(data, req.Initial)
	} else if err != badger.ErrKeyNotFound {
		return nil, err
	}

	prev := counter
	counter += req.Delta

	version := commitVersion
	if version == 0 {
		version = oldVersion + 1
	}

	expiresAt := resolveExpiry(req.TtlSeconds, req.ExpiresAt)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	envelope := store.EncodeEnvelope(int64(version), expiresAt, buf)
	entry := &badger.Entry{ Key: entryKey, Value: envelope }

	if err := txn.SetEntry(entry); err != nil {
		return nil, xerrors.Errorf("update counter error: %v", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, xerrors.Errorf("commit increment error: %v", err)
	}

	this.notify(entryKey, buf, store.WatchSet, int64(version))

	return &pb.IncrementResponse{ Previous: prev, Current: counter, Version: version }, nil

}

func (this *KeyValueStorageCtx) SetBatch(req *pb.BatchRequest, commitVersion uint64) (*pb.Status, error) {

	txn := this.db.NewTransaction(true)
	defer txn.Discard()

	// Collect notifications to fan out only after the whole batch commits.
	type change struct {
		key     []byte
		value   []byte
		version int64
	}
	pending := make([]change, 0, len(req.Records))

	// One badger transaction for the whole batch → all-or-nothing (BatchAtomic).
	// Note: bounded by badger's max transaction size; very large batches would
	// need chunking (which would forfeit atomicity — hence a bounded batch here).
	for _, record := range req.Records {

		if record.Key == nil {
			return nil, ErrorEmptyKey
		}

		entryKey, _ := EncodeKey(record.Key)

		version := commitVersion
		if version == 0 {
			// raft-off: assign per-key old+1 (reads see this txn's own pending writes).
			var oldVersion uint64
			if item, err := txn.Get(entryKey); err == nil {
				if data, err := item.ValueCopy(nil); err == nil {
					oldVersion, _ = counterFromEnvelope(data, 0)
				}
			} else if err != badger.ErrKeyNotFound {
				return nil, err
			}
			version = oldVersion + 1
		}

		expiresAt := resolveExpiry(record.TtlSeconds, record.ExpiresAt)

		envelope := store.EncodeEnvelope(int64(version), expiresAt, record.Value)
		entry := &badger.Entry{ Key: entryKey, Value: envelope, UserMeta: byte(record.Metadata) }

		if err := txn.SetEntry(entry); err != nil {
			return nil, xerrors.Errorf("batch set entry error: %v", err)
		}

		pending = append(pending, change{key: entryKey, value: record.Value, version: int64(version)})
	}

	if err := txn.Commit(); err != nil {
		return nil, xerrors.Errorf("commit batch error: %v", err)
	}

	for _, c := range pending {
		this.notify(c.key, c.value, store.WatchSet, c.version)
	}

	return &pb.Status{ Updated: true}, nil

}