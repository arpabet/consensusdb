/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package iam holds the identity model for consensusdb authentication (plan S2):
the system-tenant record layout, the identity record types with their canonical
value encoding, and the password/token primitives. Authorization (roles,
bindings, enforcement) builds on top of this in a later phase.

Identities live in the reserved system tenant so they are stored, replicated and
(once the ledger lands) audited exactly like any other data:

	major = "__system", region = "IAM"
	  user/<name>       → UserRecord            (password login)
	  sa/<name>         → ServiceAccountRecord  (token and/or mTLS login)
	  cert/<identity>   → CertIndexRecord       (cert identity → service account)
	  token/<hash>      → TokenRecord           (opaque bearer token → principal)

Bearer tokens are opaque (a kind-prefix plus a random secret, no embedded name);
authentication hashes the presented token and resolves the principal through the
token/<hash> reverse index (see token.go).

Principals follow the GCP-style convention: "user:<name>" for humans,
"serviceAccount:<name>" for workloads. Groups arrive with authorization.
*/
package iam

import (
	"strings"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/value"
)

const (
	// SystemTenant is the reserved major key owning IAM (and other system) records.
	SystemTenant = "__system"
	// Region is the system region holding identity records.
	Region = "IAM"

	UserPrefix           = "user/"
	ServiceAccountPrefix = "sa/"
	CertPrefix           = "cert/"
)

// PrincipalUser returns the principal string for a human user.
func PrincipalUser(name string) string { return "user:" + name }

// PrincipalServiceAccount returns the principal string for a workload identity.
func PrincipalServiceAccount(name string) string { return "serviceAccount:" + name }

// ParsePrincipal splits a principal "kind:name" at the first colon (so names may
// contain colons); ok=false if there is no colon.
func ParsePrincipal(principal string) (kind, name string, ok bool) {
	if i := strings.IndexByte(principal, ':'); i >= 0 {
		return principal[:i], principal[i+1:], true
	}
	return "", "", false
}

// PrincipalStorageMinor returns the system-tenant record minor for a principal
// ("user:alice" → "user/alice", "serviceAccount:x" → "sa/x"); ok=false for an
// unknown kind.
func PrincipalStorageMinor(principal string) (string, bool) {
	kind, name, ok := ParsePrincipal(principal)
	if !ok || name == "" {
		return "", false
	}
	switch kind {
	case "user":
		return UserPrefix + name, true
	case "serviceAccount":
		return ServiceAccountPrefix + name, true
	default:
		return "", false
	}
}

// Key addresses one IAM record in the system tenant.
func Key(minor string) *pb.Key {
	return &pb.Key{
		MajorKey:   []byte(SystemTenant),
		RegionName: []byte(Region),
		MinorKey:   []byte(minor),
	}
}

// UserRecord is a password-authenticated human identity.
type UserRecord struct {
	Name         string `value:"name"`
	PasswordHash string `value:"passwordHash"` // argon2id encoded string
	Admin        bool   `value:"admin"`        // initial admin flag (bindings arrive with authorization)
	Disabled     bool   `value:"disabled"`
	CreatedAt    int64  `value:"createdAt"` // unix seconds
}

// ServiceAccountRecord is a workload identity, authenticated by API token
// and/or a client-certificate identity (mTLS). Its certificate identities live in
// the cert index (cert/<identity>), the single source of truth for mTLS mapping.
type ServiceAccountRecord struct {
	Name      string `value:"name"`
	TokenHash string `value:"tokenHash"` // sha256 hex of the opaque token (its token/<hash> index key); empty = no token login
	Disabled  bool   `value:"disabled"`
	CreatedAt int64  `value:"createdAt"`
}

// CertIndexRecord maps a certificate identity (SAN URI or CN) to the principal it
// authenticates, so mTLS is a point lookup instead of a scan. It serves users and
// service accounts alike, whether the cert was issued by the built-in CA or an
// external identity was merely registered.
type CertIndexRecord struct {
	Principal string `value:"principal"` // "user:<name>" or "serviceAccount:<name>"
	Issued    bool   `value:"issued"`    // true = issued by the built-in CA here; false = externally registered
	CreatedAt int64  `value:"createdAt"`
}

// Encode packs an IAM record with the canonical value encoding (the same
// convention as the raft command payloads).
func Encode(obj interface{}) ([]byte, error) {
	v, err := value.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return value.Pack(v)
}

// Decode unpacks an IAM record produced by Encode.
func Decode(raw []byte, obj interface{}) error {
	v, err := value.Unpack(raw, true)
	if err != nil {
		return err
	}
	return value.Unmarshal(v, obj)
}
