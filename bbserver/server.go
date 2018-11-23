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
	regionStoreMap   *RegionStoreMap
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

	list := this.regionStoreMap.List()

	this.regionStoreMap.Clear()

	for _, e := range list {
		e.Value.Close();
	}

}

func NewServer(conf *Configuration) (server *BigBaggerServer, err error) {

	server = &BigBaggerServer{
		conf: conf,
		regionStoreMap: NewRegionStoreMap()}

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

			store, err := LoadBaggerDriver(filepath.Join(server.conf.DataDir, dbDir.Name()), conf)
			if err != nil {
				return nil, err
			}

			server.regionStoreMap.Put(store.GetRegion().GetName(), store)

		}

	}

	return server, nil
}

//
//
// REGION API
//
//


func (this *BigBaggerServer) Create(context context.Context, region *bbproto.Region) (response *empty.Empty, err error) {

	name := region.Name

	log.Printf("Create region: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	store, ok := this.regionStoreMap.Get(name)

	if ok {
		return new(empty.Empty), nil
	}

	store, err = NewBaggerStore(filepath.Join(this.conf.DataDir, name), region, this.conf)

	if err != nil {
		return nil, err
	}

	this.regionStoreMap.Put(name, store)

	return new(empty.Empty), nil

}

func (this *BigBaggerServer) Update(context context.Context, region *bbproto.Region) (response *empty.Empty, err error) {

	name := region.Name

	log.Printf("Alter region: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	return nil, errors.New("not supported")

}

func (this *BigBaggerServer) Delete(context context.Context, request *bbproto.String) (response *empty.Empty, err error) {

	name := request.Value

	log.Printf("Drop table: %s\n", name)

	prev, ok := this.regionStoreMap.Remove(name)

	if ok {
		prev.Close()
	}

	return new(empty.Empty), nil
}

func (this *BigBaggerServer) Get(request *bbproto.String, responseServer bbproto.RegionService_GetServer) error {

    pattern := request.Value

    if pattern == "" {
    	pattern = "*"
	}

	log.Printf("Get regions: %s\n", pattern)

    matcher, err := glob.Compile(pattern)

	if err != nil {
		return errors.New("wrong pattern")
	}


	list := this.regionStoreMap.List()

	for _, e := range list {

		if matcher.Match(e.Name) {

			responseServer.Send(e.Value.GetRegion())

		}

	}

	return nil
}

func (this *BigBaggerServer) FindRegionStore(operation *bbproto.TxOperation) IRegionStore {

	if operation.Key == nil {
		return NewErrorStore("", bbcommon.ErrorBadRequest("empty Key"))
	}

	key := operation.Key

	if key.RegionName == "" {
		return NewErrorStore("", bbcommon.ErrorBadRequest("empty Key.RegionName"))
	}

	if len(key.MajorKey) == 0 {
		return NewErrorStore(key.RegionName, bbcommon.ErrorBadRequest("replicated empty MajorKey not supported yet"))
	}

	store, ok := this.regionStoreMap.Get(key.RegionName)

	if !ok {
		return NewErrorStore(key.RegionName, bbcommon.ErrorRegionNotFound(key.RegionName))
	}

	return store
}


//
//
// RECORD API
//
//

func (this *BigBaggerServer) Execute(context context.Context, tx *bbproto.Transaction) (response *bbproto.TransactionResult, err error) {

	size := len(tx.Operations)

	response = new(bbproto.TransactionResult)
	response.Results = make([]*bbproto.TxOperationResult, 0, size)

	if size == 0 {
		return response, nil
	}

	txlist := make([]IRegionTnx, 0, size)
	txmap := make(map[string]IRegionTnx)

	for _, op := range tx.Operations {

		store := this.FindRegionStore(op)

		tx, ok := txmap[store.GetName()]

		if !ok {
			tx = store.NewTransaction()
			txmap[store.GetName()] = tx
		}

		tx.Update(bbcommon.IsUpdateOperation(op))
		txlist = append(txlist, tx)

	}

	for _, tx := range txmap {
		tx.Begin()
	}

	rollbackAll := false

	for i := 0; i < size; i = i + 1 {
		result := txlist[i].ProcessOperation(tx.Operations[i])
		response.Results = append(response.Results, result)

		if tx.AllOrNothing && !bbcommon.IsSuccessResult(result) {
			rollbackAll = true
			for i = i + 1; i < size; i = i + 1 {
				response.Results = append(response.Results, bbcommon.ErrorDriver("rollback"))
			}
			break
		}


	}

	if rollbackAll {

		for _, c := range txmap {
			c.Rollback()
		}

	} else {

		for _, c := range txmap {
			c.Commit()
		}

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
	bbproto.RegisterRegionServiceServer(this.grpcServer, this)
	bbproto.RegisterTransactionServiceServer(this.grpcServer, this)

	// Start serving requests
	return this.grpcServer.Serve(listen)

}
