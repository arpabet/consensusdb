/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/xerrors"
)

/*
Tokens are opaque bearer credentials — a kind-prefix plus a random secret, with NO
embedded name:

	sa-<64-hex>    a service-account token (long-lived)
	pat-<64-hex>   a user personal access token (expiring)

Authentication hashes the presented token and looks it up in the reverse index
token/<sha256hex(token)> → TokenRecord{principal, expiresAt}. The prefix only tells
a human what kind of token it is; the lookup is by the whole token, so the name is
never embedded, exposed, or parsed out. A 256-bit random secret makes the sha256
index key a one-way handle (a preimage attack is infeasible), so the lookup is
itself the verification.
*/

const (
	TokenPrefixServiceAccount = "sa-"
	TokenPrefixUser           = "pat-"
	TokenIndexPrefix          = "token/" // storage minor: token/<sha256hex(token)>
)

// TokenRecord is the reverse index from a token's hash to the principal it
// authenticates (and, for PATs, an expiry).
type TokenRecord struct {
	Principal string `value:"principal"` // "serviceAccount:<name>" or "user:<name>"
	ExpiresAt int64  `value:"expiresAt"` // unix seconds; 0 = never expires
}

// HashToken returns the sha256 hex of a token — its reverse-index key.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokenIndexKey is the storage minor of the reverse index for a token hash.
func TokenIndexKey(hash string) string { return TokenIndexPrefix + hash }

// NewToken mints an opaque token with the given kind-prefix (TokenPrefix*),
// returning the token (shown once, never stored) and its index-key hash.
func NewToken(prefix string) (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", xerrors.Errorf("iam: token secret: %w", err)
	}
	token = prefix + hex.EncodeToString(buf)
	return token, HashToken(token), nil
}
