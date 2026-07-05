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

func TestTokenOpaque(t *testing.T) {
	token, hash, err := NewToken(TokenPrefixServiceAccount)
	if err != nil {
		t.Fatal(err)
	}
	// The token carries only the kind-prefix, never a name.
	if !strings.HasPrefix(token, TokenPrefixServiceAccount) {
		t.Fatalf("missing prefix: %q", token)
	}
	// The stored hash is the index key, derivable from the presented token alone.
	if hash != HashToken(token) {
		t.Fatal("hash is not HashToken(token)")
	}
	if TokenIndexKey(hash) != TokenIndexPrefix+hash {
		t.Fatalf("index key: %q", TokenIndexKey(hash))
	}
	// Fresh randomness every mint.
	token2, hash2, _ := NewToken(TokenPrefixUser)
	if token == token2 || hash == hash2 {
		t.Fatal("token secret reuse")
	}
	if !strings.HasPrefix(token2, TokenPrefixUser) {
		t.Fatalf("missing PAT prefix: %q", token2)
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

	sa := &ServiceAccountRecord{Name: "robot", TokenHash: "h", CreatedAt: 7}
	raw, err = Encode(sa)
	if err != nil {
		t.Fatal(err)
	}
	gotSA := &ServiceAccountRecord{}
	if err := Decode(raw, gotSA); err != nil {
		t.Fatal(err)
	}
	if gotSA.Name != "robot" || gotSA.TokenHash != "h" || gotSA.CreatedAt != 7 {
		t.Fatalf("sa round trip mismatch: %+v", gotSA)
	}

	idx := &CertIndexRecord{Principal: PrincipalServiceAccount("robot"), Issued: true, CreatedAt: 9}
	raw, err = Encode(idx)
	if err != nil {
		t.Fatal(err)
	}
	gotIdx := &CertIndexRecord{}
	if err := Decode(raw, gotIdx); err != nil {
		t.Fatal(err)
	}
	if gotIdx.Principal != "serviceAccount:robot" || !gotIdx.Issued || gotIdx.CreatedAt != 9 {
		t.Fatalf("cert index round trip mismatch: %+v", gotIdx)
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
