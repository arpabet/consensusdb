/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"
	"go.arpabet.com/consensusdb/pkg/pb"
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
	Log     *zap.Logger     `inject:""`
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

// Touch touches the record.
func (t *KeyValueService) Touch(ctx context.Context, recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return t.Storage.Touch(recordRequest)
}

// Put puts the record.
func (t *KeyValueService) Put(ctx context.Context, recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return t.Storage.Put(recordRequest)
}

// Remove removes the record.
func (t *KeyValueService) Remove(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Status, error) {
	return t.Storage.Remove(keyRequest)
}
