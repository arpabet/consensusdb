/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import "strings"

/*
Authorization model (plan S3), GCP-shaped:

  - a fixed list of permissions (the verbs the server enforces);
  - roles = named permission lists — predefined ones compiled in (immutable),
    custom ones stored as role/<name> records;
  - bindings {role, members[]} attached at a scope: instance → tenant → region,
    inherited downward; members are principals ("user:x", "serviceAccount:x")
    or groups ("group:x", expanded one level);
  - everything lives in the system tenant next to the identities:
      group/<name>              → GroupRecord
      role/<name>               → RoleRecord (custom roles)
      policy/i                  → PolicyRecord (instance scope)
      policy/t/<tenant>         → PolicyRecord (tenant scope)
      policy/r/<tenant>/<region>→ PolicyRecord (region scope)
    (tenant/region names used in policy keys must not contain '/')

The system tenant itself is guarded by the cdb.iam.* permissions: any data-plane
operation addressing __system requires them instead of cdb.records.* — the
tenant-isolation hard floor applies to IAM data too.
*/

// Permissions the server enforces. Later phases add cdb.proofs.*, cdb.backups.*,
// cdb.tenants.*.
const (
	PermRecordsGet       = "cdb.records.get"
	PermRecordsPut       = "cdb.records.put"
	PermRecordsDelete    = "cdb.records.delete"
	PermRecordsIncrement = "cdb.records.increment"
	PermRecordsBatch     = "cdb.records.batch"
	PermRecordsEnumerate = "cdb.records.enumerate"
	PermRecordsWatch     = "cdb.records.watch"
	PermIamGet           = "cdb.iam.get"
	PermIamSet           = "cdb.iam.set"
	PermBackupsCreate    = "cdb.backups.create"
	PermBackupsRestore   = "cdb.backups.restore"
	PermClusterAdmin     = "cdb.cluster.admin"
)

// AllPermissions lists every permission the server enforces (for CLI validation
// and help text).
func AllPermissions() []string {
	return []string{
		PermRecordsGet, PermRecordsPut, PermRecordsDelete, PermRecordsIncrement,
		PermRecordsBatch, PermRecordsEnumerate, PermRecordsWatch,
		PermIamGet, PermIamSet, PermBackupsCreate, PermBackupsRestore, PermClusterAdmin,
	}
}

// IsPermission reports whether p is a known permission.
func IsPermission(p string) bool {
	for _, known := range AllPermissions() {
		if known == p {
			return true
		}
	}
	return false
}

// System-tenant minors for the authorization records.
const (
	GroupPrefix        = "group/"
	RolePrefix         = "role/"
	PolicyInstance     = "policy/i"
	PolicyTenantPrefix = "policy/t/"
	PolicyRegionPrefix = "policy/r/"
)

// PolicyTenantMinor returns the record minor holding a tenant-scope policy.
func PolicyTenantMinor(tenant string) string { return PolicyTenantPrefix + tenant }

// PolicyRegionMinor returns the record minor holding a region-scope policy.
func PolicyRegionMinor(tenant, region string) string {
	return PolicyRegionPrefix + tenant + "/" + region
}

// PrincipalGroup returns the principal string for a group (usable in bindings).
func PrincipalGroup(name string) string { return "group:" + name }

// GroupRecord names a set of member principals; binding a group grants its role
// to every member (one level — groups do not nest).
type GroupRecord struct {
	Name    string   `value:"name"`
	Members []string `value:"members"` // "user:…" / "serviceAccount:…"
}

// RoleRecord is a custom role: a named list of permissions (the GCP "text form").
type RoleRecord struct {
	Name        string   `value:"name"`
	Permissions []string `value:"permissions"`
}

// Binding grants one role to a set of members at the scope of the policy record
// holding it.
type Binding struct {
	Role    string   `value:"role"`
	Members []string `value:"members"`
}

// PolicyRecord is the set of bindings attached at one scope.
type PolicyRecord struct {
	Bindings []Binding `value:"bindings"`
}

// PredefinedRoles are compiled in, always available, and never overridable by
// stored roles.
func PredefinedRoles() map[string][]string {
	viewer := []string{PermRecordsGet, PermRecordsEnumerate, PermRecordsWatch}
	editor := append([]string{PermRecordsPut, PermRecordsDelete, PermRecordsIncrement, PermRecordsBatch}, viewer...)
	return map[string][]string{
		"roles/cdb.viewer":      viewer,
		"roles/cdb.editor":      editor,
		"roles/cdb.auditor":     viewer, // + cdb.proofs.read once the ledger lands
		"roles/cdb.tenantAdmin": append([]string{PermIamGet}, editor...),
		"roles/cdb.admin": {
			PermRecordsGet, PermRecordsPut, PermRecordsDelete, PermRecordsIncrement,
			PermRecordsBatch, PermRecordsEnumerate, PermRecordsWatch,
			PermIamGet, PermIamSet, PermBackupsCreate, PermBackupsRestore, PermClusterAdmin,
		},
	}
}

// EffectivePermission maps a data-plane permission onto the one actually
// required for the addressed tenant: operations on the system tenant require the
// cdb.iam.* permissions instead of cdb.records.*.
func EffectivePermission(perm, tenant string) string {
	if tenant != SystemTenant {
		return perm
	}
	switch perm {
	case PermRecordsGet, PermRecordsEnumerate, PermRecordsWatch:
		return PermIamGet
	default:
		return PermIamSet
	}
}

/*
Snapshot is the compiled, immutable view of the IAM records a node evaluates
against. Nodes rebuild it from the system tenant when a __system watch event
signals a change, and swap it atomically.
*/
type Snapshot struct {
	// Admins maps principals with the bootstrap admin flag → full access.
	Admins map[string]bool
	// Disabled principals are denied regardless of bindings.
	Disabled map[string]bool
	// MemberGroups maps a principal to the group principals it belongs to.
	MemberGroups map[string][]string
	// Roles maps role name → permissions (predefined + custom).
	Roles map[string][]string
	// Policies maps scope key → bindings. Scope keys: "i", "t/<tenant>",
	// "r/<tenant>/<region>".
	Policies map[string][]Binding
}

// NewSnapshot returns an empty snapshot pre-seeded with the predefined roles.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		Admins:       map[string]bool{},
		Disabled:     map[string]bool{},
		MemberGroups: map[string][]string{},
		Roles:        PredefinedRoles(),
		Policies:     map[string][]Binding{},
	}
}

// Authorize reports whether principal holds permission at the addressed scope.
// tenant=="" means an instance-wide operation (only instance bindings apply);
// region=="" means a tenant-wide operation (region bindings do not apply) — a
// grant is never broader than its binding's scope.
func (s *Snapshot) Authorize(principal, permission, tenant, region string) bool {
	if principal == "" || s.Disabled[principal] {
		return false
	}
	if s.Admins[principal] {
		return true
	}
	members := map[string]bool{principal: true}
	for _, g := range s.MemberGroups[principal] {
		members[g] = true
	}

	scopes := make([]string, 0, 3)
	if tenant != "" && region != "" {
		scopes = append(scopes, "r/"+tenant+"/"+region)
	}
	if tenant != "" {
		scopes = append(scopes, "t/"+tenant)
	}
	scopes = append(scopes, "i")

	for _, scope := range scopes {
		for _, b := range s.Policies[scope] {
			granted := false
			for _, m := range b.Members {
				if members[m] {
					granted = true
					break
				}
			}
			if !granted {
				continue
			}
			for _, p := range s.Roles[b.Role] {
				if p == permission {
					return true
				}
			}
		}
	}
	return false
}

// PolicyScopeKey converts a policy record minor into the snapshot scope key
// ("policy/i" → "i", "policy/t/acme" → "t/acme", …); ok=false for non-policy minors.
func PolicyScopeKey(minor string) (string, bool) {
	switch {
	case minor == PolicyInstance:
		return "i", true
	case strings.HasPrefix(minor, PolicyTenantPrefix):
		return "t/" + minor[len(PolicyTenantPrefix):], true
	case strings.HasPrefix(minor, PolicyRegionPrefix):
		return "r/" + minor[len(PolicyRegionPrefix):], true
	default:
		return "", false
	}
}
