/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package bbserver

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"os"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/gobwas/glob"
	"github.com/bigbagger/bigbagger/bbcommon"
)

type BigBaggerServer struct {
	grpcServer       *grpc.Server
	conf             *Configuration
	tableDriverMap   *TableDriverMap
	shuttingDown     bool
}

func (this *BigBaggerServer) Close() {

	if this == nil || this.shuttingDown {
		return
	}

	this.shuttingDown = true

	log.Println("gRPC server shutting down")

	if this.grpcServer != nil {
		this.grpcServer.Stop()
		this.grpcServer = nil
	}

	list := this.tableDriverMap.List()

	this.tableDriverMap.Clear()

	for _, e := range list {
		e.Value.Close();
	}

}

func NewServer(conf *Configuration) (server *BigBaggerServer, err error) {

	server = &BigBaggerServer{
		conf: conf,
		tableDriverMap: NewTableDriverMap()}

	log.Printf("init dataDir=%s\n", server.conf.DataDir)

	if _, err := os.Stat(server.conf.DataDir); os.IsNotExist(err) {
		return nil, err;
	}

	subDirs, err := ioutil.ReadDir(server.conf.DataDir)

	if err != nil {
		return nil, err
	}

	for _, dbDir := range subDirs {

		if dbDir.IsDir() {

			log.Printf("load dbDir=%s\n", dbDir.Name())

			driver, err := LoadBaggerDriver(filepath.Join(server.conf.DataDir, dbDir.Name()), conf)
			if err != nil {
				return nil, err
			}

			server.tableDriverMap.Put(driver.GetTable().GetName(), driver)

		}

	}

	return server, nil
}

//
//
// DATASET API
//
//


func (this *BigBaggerServer) Create(context context.Context, table *bbproto.Table) (response *empty.Empty, err error) {

	name := table.Name

	log.Printf("Create table: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	driver, ok := this.tableDriverMap.Get(name)

	if ok {
		return new(empty.Empty), nil
	}

	driver, err = NewBaggerDriver(filepath.Join(this.conf.DataDir, name), table, this.conf)

	if err != nil {
		return nil, err
	}

	this.tableDriverMap.Put(name, driver)

	return new(empty.Empty), nil

}

func (this *BigBaggerServer) Alter(context context.Context, table *bbproto.Table) (response *empty.Empty, err error) {

	name := table.Name

	log.Printf("Alter table: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	return nil, errors.New("not supported")

}

func (this *BigBaggerServer) Drop(context context.Context, request *bbproto.String) (response *empty.Empty, err error) {

	name := request.Value

	log.Printf("Drop table: %s\n", name)

	prev, ok := this.tableDriverMap.Remove(name)

	if ok {
		prev.Close()
	}

	return new(empty.Empty), nil
}

func (this *BigBaggerServer) Describe(request *bbproto.String, responseServer bbproto.TableService_DescribeServer) error {

    pattern := request.Value

    if pattern == "" {
    	pattern = "*"
	}

	log.Printf("Describe tables: %s\n", pattern)

    matcher, err := glob.Compile(pattern)

	if err != nil {
		return errors.New("wrong pattern")
	}


	list := this.tableDriverMap.List()

	for _, e := range list {

		if matcher.Match(e.Name) {

			responseServer.Send(e.Value.GetTable())

		}

	}

	return nil
}


//
//
// RECORD API
//
//

func (this *BigBaggerServer) ExecuteOperation(operation *bbproto.RecordOperation) *bbproto.RecordResult {

	if operation.Key == nil {
		return bbcommon.ErrorBadRequest("empty Key")
	}

	key := operation.Key

	if key.RegionName == "" {
		return bbcommon.ErrorBadRequest("empty Key.RegionName")
	}

	if len(key.MajorKey) == 0 {
		return bbcommon.ErrorBadRequest("replicated empty MajorKey not supported yet")
	}

	driver, ok := this.tableDriverMap.Get(key.RegionName)

	if !ok {
		return bbcommon.ErrorTableNotFound(key.RegionName)
	}

	return driver.ProcessOperation(operation)

}

func (this *BigBaggerServer) Execute(context context.Context, tnx *bbproto.Transaction) (response *bbproto.TransactionContext, err error) {

	response = new(bbproto.TransactionContext)
	response.Results = make([]*bbproto.RecordResult, 0, len(tnx.Operations))

	if len(tnx.Operations) == 0 {
		return response, nil
	}

	for _, op := range tnx.Operations {
		response.Results = append(response.Results, this.ExecuteOperation(op))
	}

	return response, nil

}


func (this *BigBaggerServer) StartServer() error {

	// start listening for grpc
	listen, err := net.Listen("tcp4", this.conf.GrpcAddress)
	if err != nil {
		log.Fatal("port is busy " + this.conf.GrpcAddress, err)
		return err
	}

	// Create new grpc bbserver
	this.grpcServer = grpc.NewServer()
	// Register services
	bbproto.RegisterTableServiceServer(this.grpcServer, this)
	bbproto.RegisterTransactionServiceServer(this.grpcServer, this)
	// Start serving requests
	return this.grpcServer.Serve(listen)

}
