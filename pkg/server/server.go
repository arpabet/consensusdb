/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
	"path"
)

type DefaultServer struct {

	grpcServer *grpc.Server
	conf       *Configuration
	log        *zap.Logger
	kv         KeyValueStorage

	shuttingDown     atomic.Bool

}


func (this *DefaultServer) Close() {

	if this == nil || !this.shuttingDown.CAS(false, true) {
		return
	}

	log.Println("server shutting down...")

	if this.grpcServer != nil {
		this.grpcServer.Stop()
		this.grpcServer = nil
	}

	this.kv.Close()

}

func NewLogger(logdir, filename string) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{
		path.Join(logdir, filename),
	}
	return cfg.Build()
}

func NewServer(conf *Configuration) (server *DefaultServer, err error) {

	log, err := NewLogger(conf.LogDir, "cdb.log")
	if err != nil {
		return nil, err
	}

	log.Info("StartServer", zap.String("dataDir", conf.DataDir))

	kv, err := OpenKeyValueStorage(conf, log);
	if err != nil {
		return nil, err
	}

	server = &DefaultServer{
		conf:           conf,
		log:            log,
		kv:             kv,
	}

	return server,nil
}

//
// Gets exact match entry
//
func (this *DefaultServer) Get(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return this.kv.Get(keyRequest)
}

//
// Gets early or equal timestamp entry
//
func (this *DefaultServer) GetRecent(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Record, error) {
	return this.kv.GetRecent(keyRequest)
}

//
// Gets range of timestamps inside a row
//
func (this *DefaultServer) GetRange(ctx context.Context, rangeRequest *pb.RangeRequest) (*pb.Block, error) {
	return this.kv.GetRange(rangeRequest)
}

//
// Gets the whole raw of records with all available timestamps with latest versions
//
func (this *DefaultServer) GetRow(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetRowServer) error {
	return this.kv.GetArea(keyRequest, MinorKeyField, response)
}

//
// Gets the whole region of records
//
func (this *DefaultServer) GetRegion(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetRegionServer) error {
	return this.kv.GetArea(keyRequest, RegionNameField, response)
}

//
// Gets the whole space of records associated with majorKey
//
func (this *DefaultServer) GetSpace(keyRequest *pb.KeyRequest, response pb.KeyValueService_GetSpaceServer) error {
	return this.kv.GetArea(keyRequest, MajorKeyField, response)
}

//
// Gets all records
//
func (this *DefaultServer) Scan(scanRequest *pb.ScanRequest, response pb.KeyValueService_ScanServer) error {
	return this.kv.Scan(scanRequest, response)
}

//
// Touches the record
//
func (this *DefaultServer) Touch(ctx context.Context,  recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return this.kv.Touch(recordRequest)
}

//
// Puts the record
//
func (this *DefaultServer) Put(ctx context.Context, recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return this.kv.Put(recordRequest)
}

//
// Remove the record
//
func (this *DefaultServer) Remove(ctx context.Context, keyRequest *pb.KeyRequest) (*pb.Status, error) {
	return this.kv.Remove(keyRequest)
}


func (this *DefaultServer) ServeGRPC() error {

	// start listening for grpc
	listen, err := net.Listen("tcp4", this.conf.GrpcAddress)
	if err != nil {
		this.log.Fatal("port is busy " + this.conf.GrpcAddress, zap.Error(err))
		return err
	}

	// Create new grpc pkg
	this.grpcServer = grpc.NewServer()

	// Register services
	pb.RegisterKeyValueServiceServer(this.grpcServer, this)

	// Start serving requests
	return this.grpcServer.Serve(listen)

}
