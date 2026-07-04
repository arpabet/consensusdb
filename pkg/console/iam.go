/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
)

/*
IAM management for the admin console: users, service accounts (application
tokens), roles, and role bindings. Reads require cdb.iam.get, writes cdb.iam.set
(enforced in http.go). Records are written the same way the `consensusdb iam` CLI
and the onboarding bootstrap write them — encoded with iam.Encode and routed
through the key-value service (which replicates via raft when enabled).
*/

// scanIAM collects every record in the system IAM region and hands each
// (minor-key, value) pair to fn.
func (t *ConsoleHandler) scanIAM(fn func(minor string, value []byte)) error {
	sender := senderFunc(func(block *pb.Block) error {
		for _, rec := range block.Record {
			if rec != nil && rec.Key != nil && len(rec.Value) > 0 {
				fn(string(rec.Key.MinorKey), rec.Value)
			}
		}
		return nil
	})
	return t.Storage.GetArea(&pb.KeyRequest{Key: &pb.Key{
		MajorKey:   []byte(iam.SystemTenant),
		RegionName: []byte(iam.Region),
	}}, server.RegionNameField, sender)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

type userOut struct {
	Name      string `json:"name"`
	Admin     bool   `json:"admin"`
	Disabled  bool   `json:"disabled"`
	CreatedAt int64  `json:"createdAt"`
}

func (t *ConsoleHandler) iamListUsers(w http.ResponseWriter) {
	var users []userOut
	err := t.scanIAM(func(minor string, value []byte) {
		if name, ok := strings.CutPrefix(minor, iam.UserPrefix); ok {
			u := &iam.UserRecord{}
			if iam.Decode(value, u) == nil {
				users = append(users, userOut{name, u.Admin, u.Disabled, u.CreatedAt})
			}
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (t *ConsoleHandler) iamCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Admin    bool   `json:"admin"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Username == "" || strings.ContainsAny(req.Username, "/ ") || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "username (no spaces or '/') and a password of at least 8 characters are required")
		return
	}
	hash, err := iam.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash password")
		return
	}
	raw, err := iam.Encode(&iam.UserRecord{Name: req.Username, PasswordHash: hash, Admin: req.Admin, CreatedAt: time.Now().Unix()})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode record")
		return
	}
	status, err := t.svc.Put(context.Background(), &pb.RecordRequest{
		Key: iam.Key(iam.UserPrefix + req.Username), Value: raw, CompareAndSet: true, Version: 0,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status == nil || !status.Updated {
		writeErr(w, http.StatusConflict, "user already exists")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"created": req.Username, "admin": req.Admin})
}

func (t *ConsoleHandler) iamDeleteUser(w http.ResponseWriter, name string) {
	if name == "" {
		writeErr(w, http.StatusBadRequest, "user name required")
		return
	}
	if _, err := t.svc.Remove(context.Background(), &pb.KeyRequest{Key: iam.Key(iam.UserPrefix + name)}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

// ---------------------------------------------------------------------------
// Service accounts (application tokens)
// ---------------------------------------------------------------------------

type saOut struct {
	Name           string   `json:"name"`
	HasToken       bool     `json:"hasToken"`
	CertIdentities []string `json:"certIdentities"`
	Disabled       bool     `json:"disabled"`
	CreatedAt      int64    `json:"createdAt"`
}

func (t *ConsoleHandler) iamListServiceAccounts(w http.ResponseWriter) {
	var accounts []saOut
	err := t.scanIAM(func(minor string, value []byte) {
		if name, ok := strings.CutPrefix(minor, iam.ServiceAccountPrefix); ok {
			sa := &iam.ServiceAccountRecord{}
			if iam.Decode(value, sa) == nil {
				accounts = append(accounts, saOut{name, sa.TokenHash != "", sa.CertIdentities, sa.Disabled, sa.CreatedAt})
			}
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan service accounts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"serviceAccounts": accounts})
}

// iamCreateServiceAccount mints a service account and its API token. The token is
// returned exactly once — it is not recoverable afterwards (only its hash is
// stored).
func (t *ConsoleHandler) iamCreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Name == "" || strings.ContainsAny(req.Name, "./ ") {
		writeErr(w, http.StatusBadRequest, "name is required and must not contain '.', '/' or spaces")
		return
	}
	token, secretHash, err := iam.GenerateToken(req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := iam.Encode(&iam.ServiceAccountRecord{Name: req.Name, TokenHash: secretHash, CreatedAt: time.Now().Unix()})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode record")
		return
	}
	status, err := t.svc.Put(context.Background(), &pb.RecordRequest{
		Key: iam.Key(iam.ServiceAccountPrefix + req.Name), Value: raw, CompareAndSet: true, Version: 0,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status == nil || !status.Updated {
		writeErr(w, http.StatusConflict, "service account already exists")
		return
	}
	// Token shown once.
	writeJSON(w, http.StatusCreated, map[string]any{"name": req.Name, "token": token})
}

func (t *ConsoleHandler) iamDeleteServiceAccount(w http.ResponseWriter, name string) {
	if name == "" {
		writeErr(w, http.StatusBadRequest, "service account name required")
		return
	}
	if _, err := t.svc.Remove(context.Background(), &pb.KeyRequest{Key: iam.Key(iam.ServiceAccountPrefix + name)}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

func (t *ConsoleHandler) iamListRoles(w http.ResponseWriter) {
	roles := map[string][]string{}
	for name, perms := range iam.PredefinedRoles() {
		roles[name] = perms
	}
	err := t.scanIAM(func(minor string, value []byte) {
		if _, ok := strings.CutPrefix(minor, iam.RolePrefix); ok {
			rr := &iam.RoleRecord{}
			if iam.Decode(value, rr) == nil {
				if _, predefined := roles[rr.Name]; !predefined {
					roles[rr.Name] = rr.Permissions
				}
			}
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan roles")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

// ---------------------------------------------------------------------------
// Role bindings (instance / tenant / region scope)
// ---------------------------------------------------------------------------

type bindingOut struct {
	Scope    string   `json:"scope"` // "" instance, "acme" tenant, "acme/USERS" region
	Role     string   `json:"role"`
	Members  []string `json:"members"`
	scopeKey string   // internal: the record minor
}

func (t *ConsoleHandler) iamListBindings(w http.ResponseWriter) {
	var out []bindingOut
	err := t.scanIAM(func(minor string, value []byte) {
		scope, ok := bindingScopeLabel(minor)
		if !ok {
			return
		}
		p := &iam.PolicyRecord{}
		if iam.Decode(value, p) == nil {
			for _, b := range p.Bindings {
				out = append(out, bindingOut{Scope: scope, Role: b.Role, Members: b.Members})
			}
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan bindings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": out})
}

// iamGrant / iamRevoke add or remove members from a role binding at a scope.
func (t *ConsoleHandler) iamChangeBinding(w http.ResponseWriter, r *http.Request, grant bool) {
	var req struct {
		Role    string   `json:"role"`
		Members []string `json:"members"`
		Tenant  string   `json:"tenant"`
		Region  string   `json:"region"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Role == "" || len(req.Members) == 0 {
		writeErr(w, http.StatusBadRequest, "role and members are required")
		return
	}
	if req.Region != "" && req.Tenant == "" {
		writeErr(w, http.StatusBadRequest, "region requires tenant")
		return
	}
	minor := bindingMinor(req.Tenant, req.Region)

	rec, err := t.svc.Get(context.Background(), &pb.KeyRequest{Key: iam.Key(minor)})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	policy := &iam.PolicyRecord{}
	if rec != nil && len(rec.Value) > 0 {
		_ = iam.Decode(rec.Value, policy)
	}
	policy.Bindings = applyBinding(policy.Bindings, req.Role, req.Members, grant)

	raw, err := iam.Encode(policy)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode policy")
		return
	}
	if _, err := t.svc.Put(context.Background(), &pb.RecordRequest{Key: iam.Key(minor), Value: raw}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scope": bindingScopeLabelOf(req.Tenant, req.Region), "role": req.Role})
}

// bindingMinor maps a (tenant, region) scope to its policy record minor.
func bindingMinor(tenant, region string) string {
	switch {
	case tenant == "":
		return iam.PolicyInstance
	case region == "":
		return iam.PolicyTenantMinor(tenant)
	default:
		return iam.PolicyRegionMinor(tenant, region)
	}
}

// bindingScopeLabel turns a policy minor into a human scope label, or ok=false.
func bindingScopeLabel(minor string) (string, bool) {
	switch {
	case minor == iam.PolicyInstance:
		return "", true
	case strings.HasPrefix(minor, iam.PolicyTenantPrefix):
		return minor[len(iam.PolicyTenantPrefix):], true
	case strings.HasPrefix(minor, iam.PolicyRegionPrefix):
		return minor[len(iam.PolicyRegionPrefix):], true
	default:
		return "", false
	}
}

func bindingScopeLabelOf(tenant, region string) string {
	switch {
	case tenant == "":
		return ""
	case region == "":
		return tenant
	default:
		return tenant + "/" + region
	}
}

// applyBinding merges or removes members for a role, dropping empty bindings.
func applyBinding(bindings []iam.Binding, role string, members []string, grant bool) []iam.Binding {
	var out []iam.Binding
	found := false
	for _, b := range bindings {
		if b.Role != role {
			out = append(out, b)
			continue
		}
		found = true
		set := map[string]bool{}
		for _, m := range b.Members {
			set[m] = true
		}
		for _, m := range members {
			set[m] = grant
			if !grant {
				delete(set, m)
			}
		}
		merged := keysOf(set)
		if len(merged) > 0 {
			out = append(out, iam.Binding{Role: role, Members: merged})
		}
	}
	if grant && !found {
		out = append(out, iam.Binding{Role: role, Members: dedup(members)})
	}
	return out
}

func keysOf(set map[string]bool) []string {
	var out []string
	for k, v := range set {
		if v {
			out = append(out, k)
		}
	}
	return out
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// decodeJSON reads a small JSON body into v, writing a 400 on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return err
	}
	return nil
}
