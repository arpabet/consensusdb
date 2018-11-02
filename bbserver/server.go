package bbserver

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/ini.v1"
	"log"
	"net"
	"bigbagger/proto/bbproto"
	"os"
	"sync"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
)

type BigBaggerServer struct {
	grpcServer   *grpc.Server
	dataDir      string

	mutex        sync.Mutex
	sets         map[string]*DatasetContext
}

func (this *BigBaggerServer) Close() {
	println("BigBagger Closing")

	if this.grpcServer != nil {
		this.grpcServer.Stop()
		this.grpcServer = nil
	}

	for _, v := range this.sets {
		v.Close();
	}

	this.sets = make(map[string]*DatasetContext)

}

func NewServer(cfg *ini.File) (server *BigBaggerServer, err error) {

	server = new(BigBaggerServer)
	server.sets = make(map[string]*DatasetContext)

	section := cfg.Section("database")

	server.dataDir = section.Key("dataDir").String()

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

			server.sets[dataset.GetName()] = dataset

		}

	}

	return server, nil
}

//
//
// DATASET API
//
//


func (this *BigBaggerServer) Create(context context.Context, request *bbproto.CreateDatasetRequest) (response *bbproto.CreateDatasetResponse, err error) {

	if request.Dataset == nil {
		return nil, errors.New("empty dataset")
	}

	name := request.Dataset.Name

	response = new(bbproto.CreateDatasetResponse)
	response.Name = name

	log.Printf("Create dataset: %s\n", name)

	this.mutex.Lock()
	defer this.mutex.Unlock()

	set, ok := this.sets[name]
	if ok {
		return response, nil
	}

	set, err = NewDataset(filepath.Join(this.dataDir, name), request.Dataset)

	if err != nil {
		return nil, err
	}

	this.sets[name] = set

	return response, nil

}

func (this *BigBaggerServer) Update(context context.Context, request *bbproto.UpdateDatasetRequest) (response *bbproto.UpdateDatasetResponse, err error) {

	if request.Dataset == nil {
		return nil, errors.New("empty dataset")
	}

	name := request.Dataset.Name

	log.Printf("Update dataset: %s\n", name)

	return nil, errors.New("not supported")

}

func (this *BigBaggerServer) Delete(context context.Context, request *bbproto.DeleteDatasetRequest) (response *bbproto.DeleteDatasetResponse, err error) {

	name := request.Name

	log.Printf("Delete dataset: %s\n", name)

	this.mutex.Lock()
	defer this.mutex.Unlock()

	dataset, ok := this.sets[name]
	if !ok {
		return nil, errors.New("dataset not found")
	}

	dataset.Close()

	delete(this.sets, name)

	response = new(bbproto.DeleteDatasetResponse)
	response.Name = name

	return response, nil
}

func (this *BigBaggerServer) List(context context.Context, request *bbproto.ListDatasetsRequest) (response *bbproto.ListDatasetsResponse, err error) {

	log.Printf("List datasets\n")

	response = new(bbproto.ListDatasetsResponse)

	this.mutex.Lock()
	defer this.mutex.Unlock()

	for k, _ := range this.sets {
		response.Name = append(response.Name, k)
	}

	return response, nil

}

func (this *BigBaggerServer) Status(context context.Context, request *bbproto.GetDatasetStatusRequest) (response *bbproto.GetDatasetStatusResponse, err error) {

	name := request.Name

	log.Printf("Get dataset status: %s\n", name)

	response = new(bbproto.GetDatasetStatusResponse)

	this.mutex.Lock()
	defer this.mutex.Unlock()

	set, ok := this.sets[name]
	if !ok {
		return nil, errors.New("dataset not found")
	}

	response.Dataset = set.dataset

	return response, nil

}

//
//
// RECORD API
//
//

func (this *BigBaggerServer) Execute(context context.Context, request *bbproto.RecordRequest) (response *bbproto.RecordResponse, err error) {

	log.Printf("Execute record dataset: %s\n", request.Token)


	response = new(bbproto.RecordResponse)

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
	bbproto.RegisterRecordServiceServer(this.grpcServer, this)
	// Start serving requests
	return this.grpcServer.Serve(listen)

}
