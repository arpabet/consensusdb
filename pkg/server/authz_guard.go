/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"

	"go.arpabet.com/consensusdb/pkg/pb"
)

/*
Authorization guards for the value-rpc data-plane handlers (plan S3). Each guard
reads the addressed (tenant, region) from the request key and checks the required
permission via the PolicyService before delegating; a denial short-circuits with
a Unauthenticated value-rpc error. When authorization is disabled (auth.enabled=
false) the PolicyService.Authorize call is a no-op, so guards add nothing.

Tenant/region come from the request key exactly as the client set them (major =
tenant, region = region), the same fields storage keys on — so a caller cannot
address one tenant while being authorized for another.
*/

// scopeOf returns the (tenant, region) a key addresses.
func scopeOf(key *pb.Key) (tenant, region string) {
	if key == nil {
		return "", ""
	}
	return string(key.MajorKey), string(key.RegionName)
}

// guardKey wraps a handler whose request is addressed by a *pb.Key accessor.
func guardKey[Req, Resp any](p *PolicyService, perm string, keyOf func(Req) *pb.Key, fn func(context.Context, Req) (Resp, error)) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, req Req) (Resp, error) {
		tenant, region := scopeOf(keyOf(req))
		if err := p.Authorize(ctx, perm, tenant, region); err != nil {
			var zero Resp
			return zero, err
		}
		return fn(ctx, req)
	}
}

func keyReqKey(r *pb.KeyRequest) *pb.Key       { return r.Key }
func recordReqKey(r *pb.RecordRequest) *pb.Key { return r.Key }
func incrementReqKey(r *pb.IncrementRequest) *pb.Key {
	return r.Key
}

// batchScope returns the (tenant, region) a batch addresses; a batch must target
// a single tenant+region (they collocate anyway), so the first record decides
// and mixed batches are rejected before any authorization decision.
func batchScope(req *pb.BatchRequest) (tenant, region string, ok bool) {
	if len(req.Records) == 0 {
		return "", "", true
	}
	tenant, region = scopeOf(req.Records[0].Key)
	for _, r := range req.Records[1:] {
		if tt, rr := scopeOf(r.Key); tt != tenant || rr != region {
			return "", "", false
		}
	}
	return tenant, region, true
}
