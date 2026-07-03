/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"testing"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/util"
	"go.uber.org/zap"
)

// With consensusdb.encryption-key set, badger encrypts at rest: the store is
// readable only when reopened with the same key, and reopening with a different
// key is rejected — proving the option actually gates the data.
func TestBadgerEncryptionAtRest(t *testing.T) {
	dir := t.TempDir()
	key, err := util.GenerateMasterKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sk := &pb.Key{MajorKey: []byte("m"), RegionName: []byte("R"), MinorKey: []byte("k")}

	open := func(encKey string) (*KeyValueStorageCtx, error) {
		conf := &Configuration{DataDir: dir, EncryptionKey: encKey}
		if err := conf.PostConstruct(); err != nil {
			return nil, err
		}
		return OpenKeyValueStorage(conf, zap.NewNop())
	}

	// Write with encryption enabled.
	kv, err := open(key)
	if err != nil {
		t.Fatalf("open encrypted: %v", err)
	}
	if _, err := kv.Put(&pb.RecordRequest{Key: sk, Value: []byte("secret")}, 1); err != nil {
		t.Fatalf("put: %v", err)
	}
	_ = kv.Close()

	// Reopen with the same key: the value is readable.
	kv2, err := open(key)
	if err != nil {
		t.Fatalf("reopen with same key: %v", err)
	}
	if rec, err := kv2.Get(&pb.KeyRequest{Key: sk}); err != nil || string(rec.Value) != "secret" {
		t.Fatalf("get = %q err=%v, want secret", rec.Value, err)
	}
	_ = kv2.Close()

	// Reopen with a different key: badger rejects it.
	other, _ := util.GenerateMasterKey()
	if kv3, err := open(other); err == nil {
		_ = kv3.Close()
		t.Fatal("opening with a wrong encryption key should fail")
	}
}
