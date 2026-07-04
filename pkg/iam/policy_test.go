/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import "testing"

func snapshotFixture() *Snapshot {
	s := NewSnapshot()
	s.Admins["user:root"] = true
	s.Disabled["user:off"] = true
	s.MemberGroups["user:carol"] = []string{"group:accounting"}
	s.Roles["roles/custom.counter"] = []string{PermRecordsIncrement}
	s.Policies["t/acme"] = []Binding{
		{Role: "roles/cdb.viewer", Members: []string{"user:alice"}},
		{Role: "roles/cdb.editor", Members: []string{"group:accounting"}},
	}
	s.Policies["r/acme/APP"] = []Binding{
		{Role: "roles/cdb.editor", Members: []string{"serviceAccount:app"}},
	}
	s.Policies["i"] = []Binding{
		{Role: "roles/custom.counter", Members: []string{"serviceAccount:metering"}},
	}
	return s
}

func TestSnapshotAuthorize(t *testing.T) {
	s := snapshotFixture()
	cases := []struct {
		name                            string
		principal, perm, tenant, region string
		want                            bool
	}{
		{"admin anywhere", "user:root", PermRecordsPut, "any", "ANY", true},
		{"admin iam", "user:root", PermIamSet, SystemTenant, Region, true},
		{"disabled denied", "user:off", PermRecordsGet, "acme", "APP", false},
		{"unauthenticated denied", "", PermRecordsGet, "acme", "APP", false},

		{"tenant viewer reads any region", "user:alice", PermRecordsGet, "acme", "APP", true},
		{"tenant viewer reads other region", "user:alice", PermRecordsGet, "acme", "OTHER", true},
		{"viewer cannot write", "user:alice", PermRecordsPut, "acme", "APP", false},
		{"tenant isolation floor", "user:alice", PermRecordsGet, "globex", "APP", false},
		{"tenant grant not instance-wide", "user:alice", PermRecordsEnumerate, "", "", false},

		{"region editor writes its region", "serviceAccount:app", PermRecordsPut, "acme", "APP", true},
		{"region editor blocked next region", "serviceAccount:app", PermRecordsPut, "acme", "OTHER", false},
		{"region grant not tenant-wide", "serviceAccount:app", PermRecordsEnumerate, "acme", "", false},

		{"group member gets group role", "user:carol", PermRecordsPut, "acme", "APP", true},
		{"group member wrong tenant", "user:carol", PermRecordsPut, "globex", "APP", false},

		{"custom role instance-wide", "serviceAccount:metering", PermRecordsIncrement, "acme", "APP", true},
		{"custom role only its perms", "serviceAccount:metering", PermRecordsGet, "acme", "APP", false},

		{"nobody without binding", "user:mallory", PermRecordsGet, "acme", "APP", false},
	}
	for _, tc := range cases {
		if got := s.Authorize(tc.principal, tc.perm, tc.tenant, tc.region); got != tc.want {
			t.Fatalf("%s: Authorize(%s,%s,%s,%s) = %v, want %v",
				tc.name, tc.principal, tc.perm, tc.tenant, tc.region, got, tc.want)
		}
	}
}

func TestEffectivePermission(t *testing.T) {
	if EffectivePermission(PermRecordsGet, "acme") != PermRecordsGet {
		t.Fatal("normal tenant must keep records perms")
	}
	if EffectivePermission(PermRecordsGet, SystemTenant) != PermIamGet {
		t.Fatal("system reads require iam.get")
	}
	if EffectivePermission(PermRecordsPut, SystemTenant) != PermIamSet {
		t.Fatal("system writes require iam.set")
	}
	if EffectivePermission(PermRecordsEnumerate, SystemTenant) != PermIamGet {
		t.Fatal("system enumerate requires iam.get")
	}
}

func TestPolicyScopeKey(t *testing.T) {
	for minor, want := range map[string]string{
		PolicyInstance:                 "i",
		PolicyTenantMinor("acme"):      "t/acme",
		PolicyRegionMinor("acme", "R"): "r/acme/R",
	} {
		got, ok := PolicyScopeKey(minor)
		if !ok || got != want {
			t.Fatalf("scope of %q = %q/%v, want %q", minor, got, ok, want)
		}
	}
	if _, ok := PolicyScopeKey(UserPrefix + "alice"); ok {
		t.Fatal("non-policy minor must not map")
	}
}
