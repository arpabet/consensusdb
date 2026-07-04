/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/store"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.uber.org/zap"
)

/*
PolicyService is the data-plane authorizer (plan S3). It compiles the IAM records
of the system tenant into an immutable iam.Snapshot, evaluates every request's
permission against it, and rebuilds the snapshot when a __system watch event
signals that identities, groups, roles or policies changed (the watch hub is fed
from the raft apply path, so every node converges on the replicated policy).

Enforcement rides the same auth.enabled switch as authentication: without
authenticated principals authorization is meaningless. Denials are logged — the
authorization half of the access audit trail started in S2.
*/
type PolicyService struct {
	Enabled bool            `value:"auth.enabled,default=false"`
	Storage KeyValueStorage `inject:""`
	Log     *zap.Logger     `inject:""`

	snap   atomic.Pointer[iam.Snapshot]
	dirty  atomic.Bool
	mu     sync.Mutex // serializes rebuilds
	cancel context.CancelFunc
}

func (t *PolicyService) BeanName() string { return "policy-service" }

func (t *PolicyService) PostConstruct() error {
	if !t.Enabled {
		return nil
	}
	if err := t.rebuild(); err != nil {
		return err
	}
	// Invalidate on any system-tenant change; the next Authorize rebuilds.
	prefix, err := EncodeKeyPrefix(&pb.Key{MajorKey: []byte(iam.SystemTenant)}, MajorKeyField)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	go func() {
		_ = t.Storage.WatchRaw(ctx, prefix, func(*store.WatchEvent) bool {
			t.dirty.Store(true)
			return true
		})
	}()
	t.Log.Info("PolicyServiceEnabled")
	return nil
}

func (t *PolicyService) Destroy() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// Authorize allows or denies permission for the principal on ctx at the
// addressed (tenant, region) scope. On the system tenant the required
// permission switches to cdb.iam.* (iam.EffectivePermission).
func (t *PolicyService) Authorize(ctx context.Context, permission, tenant, region string) error {
	// nil (unwired) or disabled ⇒ authorization is off.
	if t == nil || !t.Enabled {
		return nil
	}
	if t.dirty.Swap(false) {
		if err := t.rebuild(); err != nil {
			t.dirty.Store(true) // retry on the next request
			t.Log.Error("PolicyRebuild", zap.Error(err))
		}
	}
	principal := valuerpc.PrincipalFromContext(ctx)
	effective := iam.EffectivePermission(permission, tenant)
	if snap := t.snap.Load(); snap != nil && snap.Authorize(principal, effective, tenant, region) {
		return nil
	}
	t.Log.Warn("AuthzDenied",
		zap.String("principal", principal),
		zap.String("permission", effective),
		zap.String("tenant", tenant),
		zap.String("region", region))
	return valuerpc.NewError(valuerpc.CodeUnauthenticated,
		"permission denied: %s requires %s on %s/%s", principal, effective, tenant, region)
}

// rebuild compiles a fresh snapshot from the system tenant's IAM region.
func (t *PolicyService) rebuild() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	sender := &collectSender{}
	req := &pb.KeyRequest{Key: &pb.Key{
		MajorKey:   []byte(iam.SystemTenant),
		RegionName: []byte(iam.Region),
	}}
	if err := t.Storage.GetArea(req, RegionNameField, sender); err != nil {
		return err
	}

	snap := iam.NewSnapshot()
	for _, rec := range sender.records {
		if rec == nil || rec.Key == nil || len(rec.Value) == 0 {
			continue
		}
		minor := string(rec.Key.MinorKey)
		switch {
		case strings.HasPrefix(minor, iam.UserPrefix):
			u := &iam.UserRecord{}
			if iam.Decode(rec.Value, u) == nil {
				p := iam.PrincipalUser(u.Name)
				snap.Admins[p] = u.Admin
				snap.Disabled[p] = u.Disabled
			}
		case strings.HasPrefix(minor, iam.ServiceAccountPrefix):
			sa := &iam.ServiceAccountRecord{}
			if iam.Decode(rec.Value, sa) == nil {
				snap.Disabled[iam.PrincipalServiceAccount(sa.Name)] = sa.Disabled
			}
		case strings.HasPrefix(minor, iam.GroupPrefix):
			g := &iam.GroupRecord{}
			if iam.Decode(rec.Value, g) == nil {
				gp := iam.PrincipalGroup(g.Name)
				for _, m := range g.Members {
					snap.MemberGroups[m] = append(snap.MemberGroups[m], gp)
				}
			}
		case strings.HasPrefix(minor, iam.RolePrefix):
			r := &iam.RoleRecord{}
			if iam.Decode(rec.Value, r) == nil {
				if _, predefined := snap.Roles[r.Name]; !predefined { // predefined win
					snap.Roles[r.Name] = r.Permissions
				}
			}
		default:
			if scope, ok := iam.PolicyScopeKey(minor); ok {
				p := &iam.PolicyRecord{}
				if iam.Decode(rec.Value, p) == nil {
					snap.Policies[scope] = p.Bindings
				}
			}
		}
	}
	t.snap.Store(snap)
	return nil
}
