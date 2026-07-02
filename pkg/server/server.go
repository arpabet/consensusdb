/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"context"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/store"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

/*
KeyValueService is the gRPC service bean. It implements the generated
pb.KeyValueServiceServer and the serviongrpc.GrpcService hook (RegisterGrpc) that
registers it on the shared *grpc.Server managed by servion/grpc. All operations
delegate to the injected KeyValueStorage bean.
*/
type KeyValueService struct {
	pb.UnimplementedKeyValueServiceServer
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

func (t *KeyValueService) RegisterGrpc(srv *grpc.Server) {
	pb.RegisterKeyValueServiceServer(srv, t)
}

// Get gets exact match entry.
func (t *KeyValueService) Get(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return t.Storage.Get(keyRequest)
}

// GetRecent gets early or equal timestamp entry.
func (t *KeyValueService) GetRecent(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return t.Storage.GetRecent(keyRequest)
}

// GetRange gets a range of timestamps inside a row.
func (t *KeyValueService) GetRange(ctx context.Context, rangeRequest *pb.RangeRequest) (*pb.Block, error) {
	return t.Storage.GetRange(rangeRequest)
}

// GetRow streams the whole row of records with all available timestamps.
func (t *KeyValueService) GetRow(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetRowServer) error {
	return t.Storage.GetArea(keyRequest, MinorKeyField, response)
}

// GetRegion streams the whole region of records.
func (t *KeyValueService) GetRegion(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetRegionServer) error {
	return t.Storage.GetArea(keyRequest, RegionNameField, response)
}

// GetSpace streams the whole space of records associated with majorKey.
func (t *KeyValueService) GetSpace(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetSpaceServer) error {
	return t.Storage.GetArea(keyRequest, MajorKeyField, response)
}

// Scan streams all records.
func (t *KeyValueService) Scan(scanRequest *pb.ScanRequest, response pb.KeyValueService_ScanServer) error {
	return t.Storage.Scan(scanRequest, response)
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

// Watch streams change events for keys under the requested prefix from this
// node's local hub. Served locally (not through raft); the hub is fed by the
// apply path, so a client watching any node sees changes committed via raft.
func (t *KeyValueService) Watch(req *pb.WatchRequest, stream pb.KeyValueService_WatchServer) error {
	prefix, err := watchPrefix(req.Prefix)
	if err != nil {
		return err
	}
	return t.Storage.WatchRaw(stream.Context(), prefix, func(ev *store.WatchEvent) bool {
		key, err := DecodeKey(ev.Key)
		if err != nil {
			t.Log.Error("watch decode key", zap.Error(err))
			return true // skip this event, keep watching
		}
		changeType := pb.ChangeType_WATCH_SET
		if ev.Type == store.WatchDelete {
			changeType = pb.ChangeType_WATCH_DELETE
		}
		send := &pb.WatchEvent{
			Key:     key,
			Value:   ev.Value,
			Version: uint64(ev.Version),
			Type:    changeType,
		}
		if err := stream.Send(send); err != nil {
			return false // client disconnected or send failed: stop watching
		}
		return true
	})
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
