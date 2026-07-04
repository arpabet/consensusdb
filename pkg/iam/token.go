/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/xerrors"
)

/*
API tokens authenticate service accounts. Format:

	<service-account-name>.<64-hex-secret>

The name routes the lookup (no scan); the secret carries 32 random bytes, so a
fast sha256 at rest is sufficient (unlike low-entropy passwords). Service-account
names must not contain '.'.
*/

// GenerateToken mints a token for the named service account, returning the token
// (shown once, never stored) and the sha256 hex of its secret (stored).
func GenerateToken(saName string) (token, secretHash string, err error) {
	if strings.Contains(saName, ".") {
		return "", "", xerrors.Errorf("iam: service account name %q must not contain '.'", saName)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", xerrors.Errorf("iam: token secret: %w", err)
	}
	hexSecret := hex.EncodeToString(secret)
	return saName + "." + hexSecret, HashTokenSecret(hexSecret), nil
}

// ParseToken splits a presented token into the routing name and the secret.
func ParseToken(token string) (name, secret string, ok bool) {
	i := strings.IndexByte(token, '.')
	if i <= 0 || i == len(token)-1 {
		return "", "", false
	}
	return token[:i], token[i+1:], true
}

// HashTokenSecret returns the stored form of a token secret.
func HashTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// VerifyTokenSecret compares a presented secret against the stored hash in
// constant time.
func VerifyTokenSecret(storedHash, secret string) bool {
	got := HashTokenSecret(secret)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}
