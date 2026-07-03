/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"sort"
	"testing"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// Demonstrates how consensusdb's three key levels are used through the flat
// store.DataStore interface:
//
//	major  = tenant  (New's tenant arg) — partitions data; the same flat key in
//	                  two tenants is two independent records
//	region = region  (New's region arg) — a logical "table" within the tenant
//	minor  = the flat store key passed to Set/Get/Enumerate — the record identity
//
// So one Store addresses one tenant's one region, and enumeration is scoped to
// exactly that tenant+region.
func TestCdbKeyMappingTenantRegionKey(t *testing.T) {
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

	// major = tenant, region = "USERS", minor = the flat key.
	acme, err := cdb.New("acme", addr, "acme", "USERS")
	if err != nil {
		t.Fatalf("acme: %v", err)
	}
	defer acme.Destroy()
	globex, err := cdb.New("globex", addr, "globex", "USERS")
	if err != nil {
		t.Fatalf("globex: %v", err)
	}
	defer globex.Destroy()
	// Same tenant, a different region ("ORDERS").
	acmeOrders, err := cdb.New("acme-orders", addr, "acme", "ORDERS")
	if err != nil {
		t.Fatalf("acme-orders: %v", err)
	}
	defer acmeOrders.Destroy()

	ctx := context.Background()

	// The same flat key "alice" lives independently in each tenant (major isolates).
	mustSet(t, acme, "alice", "acme-alice")
	mustSet(t, globex, "alice", "globex-alice")
	mustSet(t, acme, "bob", "acme-bob")
	// Same tenant, different region ("table") — separate keyspace.
	mustSet(t, acmeOrders, "alice", "acme-order-1")

	if got := mustGet(t, acme, "alice"); got != "acme-alice" {
		t.Fatalf("acme/alice = %q", got)
	}
	if got := mustGet(t, globex, "alice"); got != "globex-alice" {
		t.Fatalf("globex/alice = %q (tenants not isolated by major)", got)
	}
	if got := mustGet(t, acmeOrders, "alice"); got != "acme-order-1" {
		t.Fatalf("acme/ORDERS/alice = %q (regions not isolated)", got)
	}

	// Enumeration is scoped to one tenant+region: acme/USERS sees only alice, bob.
	if keys := enumKeys(t, ctx, acme); !equal(keys, []string{"alice", "bob"}) {
		t.Fatalf("acme/USERS enumerate = %v, want [alice bob]", keys)
	}
	if keys := enumKeys(t, ctx, globex); !equal(keys, []string{"alice"}) {
		t.Fatalf("globex/USERS enumerate = %v, want [alice]", keys)
	}
	if keys := enumKeys(t, ctx, acmeOrders); !equal(keys, []string{"alice"}) {
		t.Fatalf("acme/ORDERS enumerate = %v, want [alice]", keys)
	}
}

func mustSet(t *testing.T, ds store.DataStore, key, val string) {
	t.Helper()
	if err := ds.SetRaw(context.Background(), []byte(key), []byte(val), store.NoTTL); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}

func mustGet(t *testing.T, ds store.DataStore, key string) string {
	t.Helper()
	v, err := ds.GetRaw(context.Background(), []byte(key), nil, nil, false)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	return string(v)
}

func enumKeys(t *testing.T, ctx context.Context, ds store.DataStore) []string {
	t.Helper()
	var keys []string
	if err := ds.EnumerateRaw(ctx, nil, nil, 100, true, false, func(e *store.RawEntry) bool {
		keys = append(keys, string(e.Key))
		return true
	}); err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	sort.Strings(keys)
	return keys
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
