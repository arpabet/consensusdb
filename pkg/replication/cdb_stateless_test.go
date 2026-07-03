/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// This is the property that makes staphi (or any store-based app) safe to run
// stateless across replicas: when every replica shares one consensusdb store,
// concurrent create-if-absent writes to the same key produce exactly one winner.
// staphi's UserStore.Create relies on this for email uniqueness — the regression
// test server/stores_test.go exercises it, and this proves it holds over the cdb
// provider against a live cluster (many concurrent clients = many replicas).
func TestCdbStatelessCreateIfAbsent(t *testing.T) {
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
	addr := "tcp://" + probe.Server.Addr().String()

	// N independent clients (stand-ins for N stateless replicas), all sharing the
	// one cluster store.
	const replicas = 16
	stores := make([]*cdb.Store, replicas)
	for i := range stores {
		ds, err := cdb.New(fmt.Sprintf("replica-%d", i), addr, "", "STAPHI")
		if err != nil {
			t.Fatalf("replica %d: %v", i, err)
		}
		defer ds.Destroy()
		stores[i] = ds
	}

	ctx := context.Background()
	emailKey := []byte("user/email/alice@example.com")

	// Every replica races to register the same email via create-if-absent
	// (CompareAndSet with version 0).
	var wins int64
	var wg sync.WaitGroup
	for i := 0; i < replicas; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := stores[i].CompareAndSetRaw(ctx, emailKey, []byte(fmt.Sprintf("user-%d", i)), store.NoTTL, 0)
			if err == nil && ok {
				atomic.AddInt64(&wins, 1)
			}
		}(i)
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1 (duplicate-account window on stateless replicas)", wins)
	}
	// The one committed value is a valid registration, and every replica reads it.
	val, err := stores[0].GetRaw(ctx, emailKey, nil, nil, false)
	if err != nil || !strings.HasPrefix(string(val), "user-") {
		t.Fatalf("stored = %q err=%v, want a single user-* winner", val, err)
	}
	for i, ds := range stores {
		v, err := ds.GetRaw(ctx, emailKey, nil, nil, false)
		if err != nil || string(v) != string(val) {
			t.Fatalf("replica %d reads %q (want %q) — replicas not consistent", i, v, val)
		}
	}
}
