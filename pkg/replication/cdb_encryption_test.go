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
	cryptostore "go.arpabet.com/store/middleware/crypto"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// The cdb provider composes with store's crypto middleware for client-side
// sealing: values are AES-GCM encrypted before they reach the cluster (the server
// only ever stores ciphertext), decrypted transparently on read, and Increment
// works over the sealed counter via the middleware's CAS loop.
func TestCdbProviderWithEncryption(t *testing.T) {
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
	base, err := cdb.New("plain", addr, "SEALED")
	if err != nil {
		t.Fatalf("cdb provider: %v", err)
	}
	defer base.Destroy()

	sealed, err := cryptostore.New(base, []byte("0123456789abcdef")) // AES-128
	if err != nil {
		t.Fatalf("crypto wrap: %v", err)
	}
	if !sealed.Features().Has(store.EncryptedCapability) {
		t.Fatal("sealed store should advertise Encrypted")
	}

	ctx := context.Background()

	// Write plaintext through the sealed store, read it back transparently.
	if err := sealed.SetRaw(ctx, []byte("secret"), []byte("plaintext"), store.NoTTL); err != nil {
		t.Fatalf("sealed set: %v", err)
	}
	if v, err := sealed.GetRaw(ctx, []byte("secret"), nil, nil, false); err != nil || string(v) != "plaintext" {
		t.Fatalf("sealed get = %q err=%v, want plaintext", v, err)
	}

	// The value the cluster actually stores is ciphertext (read via the base store,
	// which bypasses decryption).
	if raw, err := base.GetRaw(ctx, []byte("secret"), nil, nil, false); err != nil {
		t.Fatalf("base get: %v", err)
	} else if string(raw) == "plaintext" {
		t.Fatal("value stored in plaintext — sealing did not apply")
	}

	// Increment over a sealed counter (CAS loop over the decrypted value).
	if prev, err := sealed.IncrementRaw(ctx, []byte("ctr"), 100, 1, store.NoTTL); err != nil || prev != 100 {
		t.Fatalf("increment#1 = %d err=%v, want prev 100", prev, err)
	}
	if prev, err := sealed.IncrementRaw(ctx, []byte("ctr"), 100, 1, store.NoTTL); err != nil || prev != 101 {
		t.Fatalf("increment#2 = %d err=%v, want prev 101", prev, err)
	}
}
