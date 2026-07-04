/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"strings"
	"testing"
)

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Fatalf("unexpected hash form: %s", hash)
	}
	if !VerifyPassword(hash, "s3cret") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("garbage", "s3cret") {
		t.Fatal("garbage hash accepted")
	}
	// two hashes of the same password differ (fresh salt)
	hash2, _ := HashPassword("s3cret")
	if hash == hash2 {
		t.Fatal("salt reuse")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	token, storedHash, err := GenerateToken("robot")
	if err != nil {
		t.Fatal(err)
	}
	name, secret, ok := ParseToken(token)
	if !ok || name != "robot" {
		t.Fatalf("parse: %q %v", name, ok)
	}
	if !VerifyTokenSecret(storedHash, secret) {
		t.Fatal("valid secret rejected")
	}
	if VerifyTokenSecret(storedHash, secret+"x") {
		t.Fatal("tampered secret accepted")
	}
	if _, _, ok := ParseToken("no-separator"); ok {
		t.Fatal("malformed token parsed")
	}
	if _, _, err := GenerateToken("has.dot"); err == nil {
		t.Fatal("dotted name must be rejected")
	}
}

func TestRecordCodecRoundTrip(t *testing.T) {
	user := &UserRecord{Name: "alice", PasswordHash: "$argon2id$…", Admin: true, CreatedAt: 42}
	raw, err := Encode(user)
	if err != nil {
		t.Fatal(err)
	}
	got := &UserRecord{}
	if err := Decode(raw, got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "alice" || !got.Admin || got.CreatedAt != 42 || got.PasswordHash != user.PasswordHash {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	sa := &ServiceAccountRecord{Name: "robot", TokenHash: "h", CertIdentities: []string{"urn:cdb:sa:robot"}}
	raw, err = Encode(sa)
	if err != nil {
		t.Fatal(err)
	}
	gotSA := &ServiceAccountRecord{}
	if err := Decode(raw, gotSA); err != nil {
		t.Fatal(err)
	}
	if gotSA.Name != "robot" || len(gotSA.CertIdentities) != 1 || gotSA.CertIdentities[0] != "urn:cdb:sa:robot" {
		t.Fatalf("sa round trip mismatch: %+v", gotSA)
	}
}

func TestKeyLayout(t *testing.T) {
	k := Key(UserPrefix + "alice")
	if string(k.MajorKey) != SystemTenant || string(k.RegionName) != Region || string(k.MinorKey) != "user/alice" {
		t.Fatalf("key layout: %+v", k)
	}
	if PrincipalUser("alice") != "user:alice" || PrincipalServiceAccount("robot") != "serviceAccount:robot" {
		t.Fatal("principal forms")
	}
}
