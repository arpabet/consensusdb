/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package backup

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func roundTrip(t *testing.T, password string, size int) {
	t.Helper()
	plain := make([]byte, size)
	if _, err := rand.Read(plain); err != nil {
		t.Fatal(err)
	}

	var container bytes.Buffer
	w, err := NewWriter(&container, password)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r, err := NewReader(bytes.NewReader(container.Bytes()), password)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch (size=%d, encrypted=%v)", size, password != "")
	}
}

// Both modes round-trip at sizes around the chunk boundary, including empty.
func TestRoundTripSizes(t *testing.T) {
	sizes := []int{0, 1, 100, chunkSize - 1, chunkSize, chunkSize + 1, 3*chunkSize + 123, 5 * 1024 * 1024}
	for _, size := range sizes {
		roundTrip(t, "", size)              // plain
		roundTrip(t, "correct horse", size) // encrypted
	}
}

// Writing in many small pieces produces the same result as one big write.
func TestChunkedWrites(t *testing.T) {
	plain := bytes.Repeat([]byte("consensusdb-"), 20000) // > chunkSize
	var container bytes.Buffer
	w, _ := NewWriter(&container, "pw")
	for off := 0; off < len(plain); off += 7 {
		end := off + 7
		if end > len(plain) {
			end = len(plain)
		}
		if _, err := w.Write(plain[off:end]); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	r, _ := NewReader(bytes.NewReader(container.Bytes()), "pw")
	got, _ := io.ReadAll(r)
	if !bytes.Equal(got, plain) {
		t.Fatal("piecewise write mismatch")
	}
}

func encrypt(t *testing.T, password string, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, password)
	w.Write(plain)
	w.Close()
	return buf.Bytes()
}

func TestWrongPasswordFails(t *testing.T) {
	container := encrypt(t, "right", bytes.Repeat([]byte("x"), 5000))
	r, err := NewReader(bytes.NewReader(container), "wrong")
	if err != nil {
		t.Fatalf("reader init should succeed (fails on read): %v", err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("wrong password must fail")
	}
}

func TestTamperDetected(t *testing.T) {
	container := encrypt(t, "pw", bytes.Repeat([]byte("y"), 5000))
	container[len(container)-5] ^= 0xff // flip a ciphertext byte
	r, _ := NewReader(bytes.NewReader(container), "pw")
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("tampered ciphertext must fail the GCM tag")
	}
}

func TestTruncationDetected(t *testing.T) {
	// Two full chunks + a final chunk; dropping the tail removes the finality
	// marker, which must be detected rather than returning a partial dump.
	container := encrypt(t, "pw", bytes.Repeat([]byte("z"), 2*chunkSize+10))
	truncated := container[:len(container)-100]
	r, _ := NewReader(bytes.NewReader(truncated), "pw")
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("truncated dump must be detected")
	}
}

func TestModePasswordMismatch(t *testing.T) {
	// Encrypted dump, no password → error.
	enc := encrypt(t, "pw", []byte("secret"))
	if _, err := NewReader(bytes.NewReader(enc), ""); err == nil {
		t.Fatal("encrypted dump without a password must error")
	}
	// Plain dump, password given → error (prevents a false sense of protection).
	plain := encrypt(t, "", []byte("data"))
	if _, err := NewReader(bytes.NewReader(plain), "pw"); err == nil {
		t.Fatal("plain dump with a password must error")
	}
}

func TestNotADump(t *testing.T) {
	if _, err := NewReader(bytes.NewReader([]byte("random bytes not a dump")), ""); err == nil {
		t.Fatal("bad magic must be rejected")
	}
}
