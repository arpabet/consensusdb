/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.uber.org/zap"
)

/*
KeyValueService is the key-value operation core. The value-rpc data plane
(VrpcDataService) registers its unary methods (Get/Put/Touch/Remove/Increment/
Batch/…) as kv.* functions; streaming reads (enumerate/scan) and watch go straight
to Storage. All operations delegate to the injected KeyValueStorage bean.
*/
type KeyValueService struct {
	Storage KeyValueStorage `inject:""`
	// Replicator is present only when raft replication is wired in; writes are
	// routed through it when it reports Enabled(), otherwise they go straight
	// to local storage.
	Replicator Replicator  `inject:"optional"`
	Log        *zap.Logger `inject:""`
}

// replicates reports whether mutating ops should be committed via raft.
func (t *KeyValueService) replicates() bool {
	return t.Replicator != nil && t.Replicator.Enabled()
}

// Get gets exact match entry.
func (t *KeyValueService) Get(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return t.Storage.Get(keyRequest)
}

// GetRecent gets early or equal timestamp entry.
func (t *KeyValueService) GetRecent(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return t.Storage.GetRecent(keyRequest)
}

// watchPrefix encodes a WatchRequest prefix Key into the physical key prefix used
// by the hub. The watch depth follows which fields are set; an empty/nil key
// watches every change.
func watchPrefix(key *pb.Key) ([]byte, error) {
	if key == nil {
		return nil, nil
	}
	switch {
	case len(key.MinorKey) > 0:
		return EncodeKeyPrefix(key, MinorKeyField)
	case len(key.RegionName) > 0:
		return EncodeKeyPrefix(key, RegionNameField)
	case len(key.MajorKey) > 0:
		return EncodeKeyPrefix(key, MajorKeyField)
	default:
		return nil, nil
	}
}

// Touch touches the record. Replicated through raft when enabled.
func (t *KeyValueService) Touch(ctx context.Context, recordRequest *pb.RecordRequest) (*pb.Status, error) {
	// Compute absolute expiry once, here on the write-accepting (leader) node, so
	// every replica applies the same expiresAt rather than each computing its own.
	recordRequest.ExpiresAt = resolveExpiry(recordRequest.TtlSeconds, recordRequest.ExpiresAt)
	if t.replicates() {
		return t.Replicator.Touch(recordRequest)
	}
	// raft-off: storage assigns the version locally (commitVersion 0).
	return t.Storage.Touch(recordRequest, 0)
}

// Put puts the record. Replicated through raft when enabled.
func (t *KeyValueService) Put(ctx context.Context, recordRequest *pb.RecordRequest) (*pb.Status, error) {
	recordRequest.ExpiresAt = resolveExpiry(recordRequest.TtlSeconds, recordRequest.ExpiresAt)
	if t.replicates() {
		return t.Replicator.Put(recordRequest)
	}
	// raft-off: storage assigns the version locally (commitVersion 0).
	return t.Storage.Put(recordRequest, 0)
}

// Remove removes the record. Replicated through raft when enabled.
func (t *KeyValueService) Remove(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Status, error) {
	if t.replicates() {
		return t.Replicator.Remove(keyRequest)
	}
	return t.Storage.Remove(keyRequest)
}

// Increment atomically adds delta to a counter. Replicated through raft when enabled.
func (t *KeyValueService) Increment(ctx context.Context, req *pb.IncrementRequest) (*pb.IncrementResponse, error) {
	req.ExpiresAt = resolveExpiry(req.TtlSeconds, req.ExpiresAt)
	if t.replicates() {
		return t.Replicator.Increment(req)
	}
	// raft-off: storage assigns the version locally (commitVersion 0).
	return t.Storage.Increment(req, 0)
}

// Batch writes multiple records atomically. Replicated through raft when enabled.
func (t *KeyValueService) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.Status, error) {
	// Stamp absolute expiry on every record once, on the leader.
	for _, record := range req.Records {
		record.ExpiresAt = resolveExpiry(record.TtlSeconds, record.ExpiresAt)
	}
	if t.replicates() {
		return t.Replicator.Batch(req)
	}
	// raft-off: storage assigns the version locally (commitVersion 0).
	return t.Storage.SetBatch(req, 0)
}
