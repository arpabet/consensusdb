package bbserver

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
	"bigbagger/proto/bbproto"
	"os"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/gobwas/glob"
	"bigbagger/bbcommon"
)

type BigBaggerServer struct {
	grpcServer   *grpc.Server
	dataDir      string
	sets         *DatasetMap
}

func (this *BigBaggerServer) Close() {
	println("BigBagger Closing")

	if this.grpcServer != nil {
		this.grpcServer.Stop()
		this.grpcServer = nil
	}

	list := this.sets.List()

	this.sets.Clear()

	for _, e := range list {
		e.Value.Close();
	}

}

func NewServer(dataDir string) (server *BigBaggerServer, err error) {

	server = new(BigBaggerServer)
	server.sets = NewDatasetMap()
	server.dataDir = dataDir

	log.Printf("init dataDir=%s\n", server.dataDir)

	if _, err := os.Stat(server.dataDir); os.IsNotExist(err) {
		return nil, err;
	}

	subDirs, err := ioutil.ReadDir(server.dataDir)

	if err != nil {
		return nil, err
	}

	for _, dbDir := range subDirs {

		if dbDir.IsDir() {

			log.Printf("load dbDir=%s\n", dbDir.Name())

			dataset, err := LoadDataset(filepath.Join(server.dataDir, dbDir.Name()))
			if err != nil {
				return nil, err
			}

			server.sets.Put(dataset.GetName(), dataset)

		}

	}

	return server, nil
}

//
//
// DATASET API
//
//


func (this *BigBaggerServer) Create(context context.Context, dataset *bbproto.Dataset) (response *empty.Empty, err error) {

	name := dataset.Name

	log.Printf("Create dataset: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	set, ok := this.sets.Get(name)

	if ok {
		return new(empty.Empty), nil
	}

	set, err = NewDataset(filepath.Join(this.dataDir, name), dataset)

	if err != nil {
		return nil, err
	}

	this.sets.Put(name, set)

	return new(empty.Empty), nil

}

func (this *BigBaggerServer) Update(context context.Context, dataset *bbproto.Dataset) (response *empty.Empty, err error) {

	name := dataset.Name

	log.Printf("Update dataset: %s\n", name)

	if name == "" {
		return nil, errors.New("empty name")
	}

	return nil, errors.New("not supported")

}

func (this *BigBaggerServer) Delete(context context.Context, request *bbproto.String) (response *empty.Empty, err error) {

	name := request.Value

	log.Printf("Delete dataset: %s\n", name)

	prev, ok := this.sets.Remove(name)

	if ok {
		prev.Close()
	}

	return new(empty.Empty), nil
}

func (this *BigBaggerServer) Get(request *bbproto.String, responseServer bbproto.DatasetService_GetServer) error {

    pattern := request.Value

    if pattern == "" {
    	pattern = "*"
	}

	log.Printf("Get datasets: %s\n", pattern)

    matcher, err := glob.Compile(pattern)

	if err != nil {
		return errors.New("wrong pattern")
	}


	list := this.sets.List()

	for _, e := range list {

		if matcher.Match(e.Key) {

			responseServer.Send(e.Value.dataset)

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

	if len(key.SetName) == 0 {
		return bbcommon.ErrorBadRequest("empty Key.SetName")
	}

	if len(key.RecordKey) == 0 {
		return bbcommon.ErrorBadRequest("empty Key.RecordKey")
	}

	set, ok := this.sets.Get(key.SetName)

	if !ok {
		return bbcommon.ErrorDatasetNotFound(key.SetName)
	}

	return set.ProcessOperation(operation)

}

func (this *BigBaggerServer) Execute(context context.Context, tnx *bbproto.Transaction) (response *bbproto.TransactionContext, err error) {

	response = new(bbproto.TransactionContext)
	response.Results = make([]*bbproto.RecordResult, 0, len(tnx.Operations))

	log.Printf("Execute record dataset\n")

	if len(tnx.Operations) == 0 {
		return response, nil
	}

	for i, op := range tnx.Operations {
		response.Results[i] = this.ExecuteOperation(op)
	}

	return response, nil

}


func (this *BigBaggerServer) StartServer(grpcAddress string) error {

	// start listening for grpc
	listen, err := net.Listen("tcp4", grpcAddress)
	if err != nil {
		log.Fatal("port is busy " + grpcAddress, err)
		return err
	}

	// Create new grpc bbserver
	this.grpcServer = grpc.NewServer()
	// Register services
	bbproto.RegisterDatasetServiceServer(this.grpcServer, this)
	bbproto.RegisterTransactionServiceServer(this.grpcServer, this)
	// Start serving requests
	return this.grpcServer.Serve(listen)

}
