/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"testing"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// The store.DataStore cdb provider works end-to-end against a live consensusdb
// value-rpc data plane: point ops, CAS, increment, enumerate, and remove all round
// through the wire. This is the proof that an application can go stateless by
// swapping its embedded engine for this provider.
func TestCdbProviderAgainstServer(t *testing.T) {
	tmp := t.TempDir()

	probe := &vrpcDataProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.VrpcDataService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)

	glueCtx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer glueCtx.Close()

	ds, err := cdb.New("test", "tcp://"+probe.Server.Addr().String(), "STORE")
	if err != nil {
		t.Fatalf("cdb provider: %v", err)
	}
	defer ds.Destroy()

	// This provider is interchangeable for these capabilities.
	if !ds.Features().Has(store.TTLCapability | store.AtomicCapability | store.WatchCapability | store.BatchAtomicCapability) {
		t.Fatalf("features = %v", ds.Features())
	}

	ctx := context.Background()

	// SetRaw / GetRaw.
	if err := ds.SetRaw(ctx, []byte("k1"), []byte("v1"), store.NoTTL); err != nil {
		t.Fatalf("set: %v", err)
	}
	val, err := ds.GetRaw(ctx, []byte("k1"), nil, nil, false)
	if err != nil || string(val) != "v1" {
		t.Fatalf("get = %q err=%v, want v1", val, err)
	}

	// A missing key: not found, no error (required=false).
	if v, err := ds.GetRaw(ctx, []byte("missing"), nil, nil, false); err != nil || v != nil {
		t.Fatalf("get missing = %q err=%v, want nil", v, err)
	}

	// CompareAndSet against the read-back version.
	var ver int64
	if _, err := ds.GetRaw(ctx, []byte("k1"), nil, &ver, false); err != nil {
		t.Fatalf("get version: %v", err)
	}
	if ok, err := ds.CompareAndSetRaw(ctx, []byte("k1"), []byte("v2"), store.NoTTL, ver); err != nil || !ok {
		t.Fatalf("cas correct version = %v err=%v, want true", ok, err)
	}
	if ok, err := ds.CompareAndSetRaw(ctx, []byte("k1"), []byte("v3"), store.NoTTL, ver); err != nil || ok {
		t.Fatalf("cas stale version = %v err=%v, want false", ok, err)
	}

	// Increment returns the previous value.
	if prev, err := ds.IncrementRaw(ctx, []byte("counter"), 10, 5, store.NoTTL); err != nil || prev != 10 {
		t.Fatalf("increment = %d err=%v, want prev 10", prev, err)
	}

	// Batch + enumerate under a prefix.
	if err := ds.SetBatchRaw(ctx, []store.RawEntry{
		{Key: []byte("p/a"), Value: []byte("a")},
		{Key: []byte("p/b"), Value: []byte("b")},
	}); err != nil {
		t.Fatalf("batch: %v", err)
	}
	seen := map[string]string{}
	if err := ds.EnumerateRaw(ctx, []byte("p/"), nil, 100, false, false, func(e *store.RawEntry) bool {
		seen[string(e.Key)] = string(e.Value)
		return true
	}); err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if len(seen) != 2 || seen["p/a"] != "a" || seen["p/b"] != "b" {
		t.Fatalf("enumerate saw %v, want {p/a:a, p/b:b}", seen)
	}

	// Remove.
	if err := ds.RemoveRaw(ctx, []byte("k1")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if v, err := ds.GetRaw(ctx, []byte("k1"), nil, nil, false); err != nil || v != nil {
		t.Fatalf("get after remove = %q err=%v, want nil", v, err)
	}
}
