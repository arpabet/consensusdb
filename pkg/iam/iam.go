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

Principals follow the GCP-style convention: "user:<name>" for humans,
"serviceAccount:<name>" for workloads. Groups arrive with authorization.
*/
package iam

import (
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/value"
)

const (
	// SystemTenant is the reserved major key owning IAM (and other system) records.
	SystemTenant = "__system"
	// Region is the system region holding identity records.
	Region = "IAM"

	UserPrefix           = "user/"
	ServiceAccountPrefix = "sa-"
	CertPrefix           = "cert/"
)

// PrincipalUser returns the principal string for a human user.
func PrincipalUser(name string) string { return "user:" + name }

// PrincipalServiceAccount returns the principal string for a workload identity.
func PrincipalServiceAccount(name string) string { return "serviceAccount:" + name }

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
// and/or a client-certificate identity (mTLS).
type ServiceAccountRecord struct {
	Name           string   `value:"name"`
	TokenHash      string   `value:"tokenHash"`      // sha256 hex of the token secret; empty = no token login
	CertIdentities []string `value:"certIdentities"` // SAN URIs (or CN) that map to this account
	Disabled       bool     `value:"disabled"`
	CreatedAt      int64    `value:"createdAt"`
}

// CertIndexRecord points a certificate identity at its service account, so mTLS
// authentication is a point lookup instead of a scan.
type CertIndexRecord struct {
	ServiceAccount string `value:"serviceAccount"`
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
