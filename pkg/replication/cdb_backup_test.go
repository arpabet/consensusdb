/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
)

// Full round trip: seed a source node, back it up (encrypted) to a file over the
// admin stream, restore into a FRESH node, and confirm the data survived. Also
// covers the wrong-password failure.
func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dumpDir := t.TempDir()
	dump := filepath.Join(dumpDir, "cluster.dump")
	const password = "backup-pass-123"

	// --- source node: seed data ---
	srcAddr, srcCtx := startNode(t, t.TempDir())
	defer srcCtx.Close()

	src, err := cdb.New("src", srcAddr, "acme", "USERS")
	if err != nil {
		t.Fatalf("src store: %v", err)
	}
	want := map[string]string{}
	for i := 0; i < 200; i++ {
		k := fmt.Sprintf("user-%03d", i)
		v := fmt.Sprintf("value-%03d-payload", i)
		if err := src.SetRaw(ctx, []byte(k), []byte(v), store.NoTTL); err != nil {
			t.Fatalf("seed set: %v", err)
		}
		want[k] = v
	}
	_ = src.Destroy()

	// --- back up the source over the admin stream, encrypted, to a file ---
	adminSrc := valueclient.NewClient(srcAddr, "")
	if err := adminSrc.Connect(); err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	version, err := backup.Backup(ctx, adminSrc, 0, dump, password, backup.S3Config{})
	adminSrc.Close()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if version == 0 {
		t.Fatal("backup reported version 0")
	}

	// A wrong password must fail to restore (does not silently produce garbage).
	freshBad, freshBadCtx := startNode(t, t.TempDir())
	adminBad := valueclient.NewClient(freshBad, "")
	_ = adminBad.Connect()
	if err := backup.Restore(ctx, adminBad, dump, "wrong-password", backup.S3Config{}); err == nil {
		t.Fatal("restore with wrong password must fail")
	}
	adminBad.Close()
	freshBadCtx.Close()

	// --- restore into a fresh node ---
	freshDir := t.TempDir()
	freshAddr, freshCtx := startNode(t, freshDir)
	defer freshCtx.Close()

	adminFresh := valueclient.NewClient(freshAddr, "")
	if err := adminFresh.Connect(); err != nil {
		t.Fatalf("fresh admin connect: %v", err)
	}
	if err := backup.Restore(ctx, adminFresh, dump, password, backup.S3Config{}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	adminFresh.Close()

	// --- verify every record survived on the fresh node ---
	restored, err := cdb.New("fresh", freshAddr, "acme", "USERS")
	if err != nil {
		t.Fatalf("fresh store: %v", err)
	}
	defer restored.Destroy()
	for k, v := range want {
		got, err := restored.GetRaw(ctx, []byte(k), nil, nil, false)
		if err != nil || string(got) != v {
			t.Fatalf("restored[%s] = %q err=%v, want %q", k, got, err, v)
		}
	}
}

// startNode spins a single-node consensusdb with the admin surface and returns
// its data-plane address. (Local helper so the test file is self-contained.)
func startNode(t *testing.T, dir string) (addr string, glueCtx glue.Container) {
	t.Helper()
	probe := &authProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     dir,
			"application.data.dir":     dir,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.AuthService{},
		&server.PolicyService{},
		&server.VrpcDataService{},
		&server.AdminService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)
	gctx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	return "tcp://" + probe.Server.Addr().String(), gctx
}
