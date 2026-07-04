/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package backup

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
)

func TestParseS3(t *testing.T) {
	cases := []struct {
		url           string
		bucket, key   string
		isS3, wantErr bool
	}{
		{"s3://backups/consensusdb/2026-07-03.dump", "backups", "consensusdb/2026-07-03.dump", true, false},
		{"s3://b/k", "b", "k", true, false},
		{"/var/backups/a.dump", "", "", false, false},
		{"file:///var/a.dump", "", "", false, false},
		{"s3://onlybucket", "", "", false, true},
		{"s3://bucket/", "", "", false, true},
	}
	for _, c := range cases {
		b, k, ok, err := parseS3(c.url)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v want err=%v", c.url, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if ok != c.isS3 || b != c.bucket || k != c.key {
			t.Fatalf("%s: got (%q,%q,%v) want (%q,%q,%v)", c.url, b, k, ok, c.bucket, c.key, c.isS3)
		}
	}
}

// The file sink/source round-trip a dump through OpenSink/OpenSource, creating
// parent directories as needed.
func TestFileSinkSource(t *testing.T) {
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "nested", "dir", "cluster.dump")
	payload := bytes.Repeat([]byte("backup-bytes-"), 1000)

	w, err := OpenSink(ctx, dest, S3Config{})
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r, err := OpenSource(ctx, dest, S3Config{})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("round trip mismatch (err=%v)", err)
	}
}

// End-to-end: an encrypted dump written to a file sink reads back to the exact
// bytes through the source + decrypting reader.
func TestSinkEncryptRoundTrip(t *testing.T) {
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "enc.dump")
	payload := bytes.Repeat([]byte("secret-"), 20000)

	sink, _ := OpenSink(ctx, dest, S3Config{})
	w, _ := NewWriter(sink, "hunter2")
	w.Write(payload)
	w.Close()
	sink.Close()

	src, _ := OpenSource(ctx, dest, S3Config{})
	defer src.Close()
	r, err := NewReader(src, "hunter2")
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	got, _ := io.ReadAll(r)
	if !bytes.Equal(got, payload) {
		t.Fatal("encrypted file dump mismatch")
	}
}
