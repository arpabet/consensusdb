/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// End-to-end authorization: a service account bound to roles/cdb.editor on tenant
// "acme" writes there and only there; a viewer reads but cannot write; the
// tenant-isolation floor holds; a binding added at runtime is picked up via the
// __system watch (no restart); and a non-admin cannot write IAM records.
func TestCdbAuthorizationEnforced(t *testing.T) {
	tmp := t.TempDir()
	probe := &authProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
			"auth.enabled":             "true",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.AuthService{},
		&server.PolicyService{},
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

	writerTok := seedSA(t, probe.Storage, "writer")
	readerTok := seedSA(t, probe.Storage, "reader")

	// writer=editor, reader=viewer, both on tenant "acme". Seeding through storage
	// notifies the __system watch, so the PolicyService reloads.
	seedIdentity(t, probe.Storage, iam.PolicyTenantMinor("acme"), &iam.PolicyRecord{Bindings: []iam.Binding{
		{Role: "roles/cdb.editor", Members: []string{iam.PrincipalServiceAccount("writer")}},
		{Role: "roles/cdb.viewer", Members: []string{iam.PrincipalServiceAccount("reader")}},
	}})

	ctx := context.Background()
	writer, err := cdb.NewMulti("w", addr, cdb.WithCredential(cdb.TokenCredential(writerTok)))
	if err != nil {
		t.Fatalf("writer connect: %v", err)
	}
	defer writer.Destroy()
	reader, err := cdb.NewMulti("r", addr, cdb.WithCredential(cdb.TokenCredential(readerTok)))
	if err != nil {
		t.Fatalf("reader connect: %v", err)
	}
	defer reader.Destroy()

	acmeW := writer.Region("acme", "APP")
	acmeR := reader.Region("acme", "APP")
	globexW := writer.Region("globex", "APP")

	// editor writes acme (retried until the watch-driven reload lands the binding).
	until(t, 3*time.Second, "editor write acme", func() bool {
		return acmeW.SetRaw(ctx, []byte("k"), []byte("v"), store.NoTTL) == nil
	})

	// From here the snapshot is loaded, so the rest is deterministic.
	if got, err := acmeW.GetRaw(ctx, []byte("k"), nil, nil, false); err != nil || string(got) != "v" {
		t.Fatalf("editor read acme = %q err=%v", got, err)
	}
	if got, err := acmeR.GetRaw(ctx, []byte("k"), nil, nil, false); err != nil || string(got) != "v" {
		t.Fatalf("viewer read acme = %q err=%v", got, err)
	}
	if err := acmeR.SetRaw(ctx, []byte("k"), []byte("x"), store.NoTTL); err == nil {
		t.Fatal("viewer must not write")
	}
	// tenant-isolation floor: writer has no binding on globex.
	if err := globexW.SetRaw(ctx, []byte("k"), []byte("v"), store.NoTTL); err == nil {
		t.Fatal("editor on acme must not write globex")
	}
	if _, err := globexW.GetRaw(ctx, []byte("k"), nil, nil, false); err == nil {
		t.Fatal("editor on acme must not read globex")
	}

	// A non-admin (writer is only an acme editor) cannot write IAM records: the
	// __system tenant requires cdb.iam.set, which no binding grants here.
	sys := writer.Region(iam.SystemTenant, iam.Region)
	if err := sys.SetRaw(ctx, []byte(iam.RolePrefix+"evil"), []byte("x"), store.NoTTL); err == nil {
		t.Fatal("non-admin wrote to __system — iam.set floor not enforced")
	}

	// Runtime policy change: grant reader editor on globex, then it must be able
	// to write there — proving the __system watch reloads without a restart.
	seedIdentity(t, probe.Storage, iam.PolicyTenantMinor("globex"), &iam.PolicyRecord{Bindings: []iam.Binding{
		{Role: "roles/cdb.editor", Members: []string{iam.PrincipalServiceAccount("reader")}},
	}})
	globexR := reader.Region("globex", "APP")
	until(t, 3*time.Second, "runtime binding takes effect", func() bool {
		return globexR.SetRaw(ctx, []byte("k2"), []byte("v2"), store.NoTTL) == nil
	})
}

func seedSA(t *testing.T, storage server.KeyValueStorage, name string) string {
	t.Helper()
	token, hash, err := iam.GenerateToken(name)
	if err != nil {
		t.Fatal(err)
	}
	seedIdentity(t, storage, iam.ServiceAccountPrefix+name,
		&iam.ServiceAccountRecord{Name: name, TokenHash: hash, CreatedAt: time.Now().Unix()})
	return token
}

// until retries op until it returns true or the deadline elapses.
func until(t *testing.T, timeout time.Duration, what string, op func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if op() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
