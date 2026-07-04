/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// The native MultiDataStore: ONE connection serves many (tenant, region) views —
// the original cdb access model (MajorKey + RegionName per request) through the
// store interface. Views must be isolated (same minor key in different tenants/
// regions = different records), ordered enumeration stays per-view, watch is
// scoped per view, and a view is a struct — not a dial — so per-profile views
// (the Webby pattern) are free.
func TestCdbMultiRegionSharedConnection(t *testing.T) {
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

	multi, err := cdb.NewMulti("m", addr)
	if err != nil {
		t.Fatalf("dial multi: %v", err)
	}
	defer multi.Destroy()
	if !multi.Features().Has(store.TTLCapability | store.AtomicCapability | store.OrderedCapability | store.WatchCapability) {
		t.Fatalf("multi features = %v", multi.Features())
	}

	ctx := context.Background()
	aliceUsers := multi.Region("alice", "USERS")
	aliceJobs := multi.Region("alice", "JOBS")
	bobUsers := multi.Region("bob", "USERS")

	// Same minor key, three views: three independent records over one connection.
	for view, val := range map[store.DataStore]string{aliceUsers: "au", aliceJobs: "aj", bobUsers: "bu"} {
		if err := view.SetRaw(ctx, []byte("k1"), []byte(val), store.NoTTL); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	for view, val := range map[store.DataStore]string{aliceUsers: "au", aliceJobs: "aj", bobUsers: "bu"} {
		got, err := view.GetRaw(ctx, []byte("k1"), nil, nil, false)
		if err != nil || string(got) != val {
			t.Fatalf("get = %q err=%v, want %q", got, err, val)
		}
	}

	// Ordered enumeration is scoped to the view.
	_ = aliceUsers.SetRaw(ctx, []byte("a0"), []byte("y"), store.NoTTL)
	_ = aliceUsers.SetRaw(ctx, []byte("z9"), []byte("z"), store.NoTTL)
	var keys []string
	if err := aliceUsers.EnumerateRaw(ctx, nil, nil, 100, false, false, func(e *store.RawEntry) bool {
		keys = append(keys, string(e.Key))
		return true
	}); err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if len(keys) != 3 || keys[0] != "a0" || keys[1] != "k1" || keys[2] != "z9" {
		t.Fatalf("alice/USERS enumerate = %v, want [a0 k1 z9]", keys)
	}

	// A view's watch sees its own region only — over the same shared connection.
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan *store.WatchEvent, 8)
	go func() {
		_ = aliceJobs.WatchRaw(wctx, nil, func(ev *store.WatchEvent) bool {
			events <- ev
			return true
		})
	}()
	time.Sleep(200 * time.Millisecond) // let the server-side watch subscribe

	_ = bobUsers.SetRaw(ctx, []byte("noise"), []byte("x"), store.NoTTL)
	_ = aliceJobs.SetRaw(ctx, []byte("job-7"), []byte("hired"), store.NoTTL)

	select {
	case ev := <-events:
		if string(ev.Key) != "job-7" || string(ev.Value) != "hired" {
			t.Fatalf("watch delivered %q=%q, want job-7=hired", ev.Key, ev.Value)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no watch event on the alice/JOBS view")
	}
	select {
	case ev := <-events:
		t.Fatalf("foreign event leaked into view: %q", ev.Key)
	case <-time.After(200 * time.Millisecond):
	}

	// Views are borrowed: destroying one must not tear down the shared connection.
	if err := aliceUsers.Destroy(); err != nil {
		t.Fatalf("view destroy: %v", err)
	}
	if got, err := aliceJobs.GetRaw(ctx, []byte("k1"), nil, nil, false); err != nil || string(got) != "aj" {
		t.Fatalf("connection died with a borrowed view: %q err=%v", got, err)
	}
}
