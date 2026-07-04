/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"fmt"
	"strings"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
Authorization CLI (plan S3): manage custom roles, groups, and role bindings at
the instance / tenant / region scopes. All records live in the system tenant and
are written over the data plane like identities (idempotent read-modify-write on
policy records).
*/

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// IamRoleAddCommand creates or replaces a custom role (a named permission list).
type IamRoleAddCommand struct {
	Parent      cligo.CliGroup `cli:"group=iam"`
	Name        string         `cli:"argument=name,required"`
	Permissions string         `cli:"option=permissions,default=,help=comma-separated permissions (e.g. cdb.records.get,cdb.records.put)"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamRoleAddCommand) Command() string { return "role-add" }

func (t *IamRoleAddCommand) Help() (string, string) {
	return "create/replace a custom role (permission list)",
		"Permissions: " + strings.Join(iam.AllPermissions(), ", ")
}

func (t *IamRoleAddCommand) Run(ctx context.Context) error {
	perms := splitCSV(t.Permissions)
	for _, p := range perms {
		if !iam.IsPermission(p) {
			return xerrors.Errorf("unknown permission %q (see --help)", p)
		}
	}
	record, err := iam.Encode(&iam.RoleRecord{Name: t.Name, Permissions: perms})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		if err := iam.PutRecord(ctx, cli, iam.RolePrefix+t.Name, record); err != nil {
			return err
		}
		fmt.Printf("role %q set: %s\n", t.Name, strings.Join(perms, ", "))
		return nil
	})
}

// IamGroupSetCommand creates or replaces a group's membership.
type IamGroupSetCommand struct {
	Parent  cligo.CliGroup `cli:"group=iam"`
	Name    string         `cli:"argument=name,required"`
	Members string         `cli:"option=members,default=,help=comma-separated principals (user:alice,serviceAccount:app)"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamGroupSetCommand) Command() string { return "group-set" }

func (t *IamGroupSetCommand) Help() (string, string) {
	return "create/replace a group's members", ""
}

func (t *IamGroupSetCommand) Run(ctx context.Context) error {
	members := splitCSV(t.Members)
	for _, m := range members {
		if !strings.HasPrefix(m, "user:") && !strings.HasPrefix(m, "serviceAccount:") {
			return xerrors.Errorf("member %q must be user:… or serviceAccount:…", m)
		}
	}
	record, err := iam.Encode(&iam.GroupRecord{Name: t.Name, Members: members})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		if err := iam.PutRecord(ctx, cli, iam.GroupPrefix+t.Name, record); err != nil {
			return err
		}
		fmt.Printf("group %q set: %s\n", t.Name, strings.Join(members, ", "))
		return nil
	})
}

// IamBindingAddCommand adds members to a role binding at a scope.
type IamBindingAddCommand struct {
	Parent  cligo.CliGroup `cli:"group=iam"`
	Role    string         `cli:"argument=role,required"`
	Members string         `cli:"option=members,default=,help=comma-separated principals/groups (user:alice,group:accounting)"`
	Tenant  string         `cli:"option=tenant,default=,help=tenant (major key) scope; empty = instance-wide"`
	Region  string         `cli:"option=region,default=,help=region scope within the tenant; empty = whole tenant"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamBindingAddCommand) Command() string { return "binding-add" }

func (t *IamBindingAddCommand) Help() (string, string) {
	return "grant a role to members at a scope (instance/tenant/region)", ""
}

func (t *IamBindingAddCommand) Run(ctx context.Context) error {
	if t.Region != "" && t.Tenant == "" {
		return xerrors.New("--region requires --tenant")
	}
	members := splitCSV(t.Members)
	if len(members) == 0 {
		return xerrors.New("--members is required")
	}
	minor, scopeDesc := bindingMinor(t.Tenant, t.Region)

	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		policy := &iam.PolicyRecord{}
		if _, err := iam.GetRecord(ctx, cli, minor, policy); err != nil {
			return err
		}
		policy.Bindings = mergeBinding(policy.Bindings, t.Role, members)
		record, err := iam.Encode(policy)
		if err != nil {
			return err
		}
		if err := iam.PutRecord(ctx, cli, minor, record); err != nil {
			return err
		}
		fmt.Printf("bound %q to %s at %s\n", t.Role, strings.Join(members, ", "), scopeDesc)
		return nil
	})
}

// bindingMinor returns the policy record minor and a human scope description.
func bindingMinor(tenant, region string) (minor, desc string) {
	switch {
	case tenant == "":
		return iam.PolicyInstance, "instance"
	case region == "":
		return iam.PolicyTenantMinor(tenant), "tenant " + tenant
	default:
		return iam.PolicyRegionMinor(tenant, region), "tenant " + tenant + " region " + region
	}
}

// mergeBinding adds members to the binding for role (creating it if absent),
// de-duplicating members.
func mergeBinding(bindings []iam.Binding, role string, add []string) []iam.Binding {
	for i := range bindings {
		if bindings[i].Role == role {
			bindings[i].Members = dedup(append(bindings[i].Members, add...))
			return bindings
		}
	}
	return append(bindings, iam.Binding{Role: role, Members: dedup(add)})
}

func dedup(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
