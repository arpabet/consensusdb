package bbserver

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/ini.v1"
	"log"
	"net"
	"bigbagger/proto/bbproto"
	"os"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/ptypes/empty"
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

func NewServer(cfg *ini.File) (server *BigBaggerServer, err error) {

	server = new(BigBaggerServer)
	server.sets = NewDatasetMap()

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

	return nil, errors.New("not supported")

}

func (this *BigBaggerServer) Delete(context context.Context, request *bbproto.Name) (response *empty.Empty, err error) {

	name := request.Name

	log.Printf("Delete dataset: %s\n", name)

	prev, ok := this.sets.Remove(name)

	if ok {
		prev.Close()
	}

	return new(empty.Empty), nil
}

func (this *BigBaggerServer) List(context context.Context, request *empty.Empty) (response *bbproto.NameList, err error) {

	log.Printf("List datasets\n")

	response = new(bbproto.NameList)

	for _, e := range this.sets.List() {
		response.Name = append(response.Name, e.Key)
	}

	return response, nil

}

func (this *BigBaggerServer) Status(context context.Context, request *bbproto.Name) (response *bbproto.Dataset, err error) {

	name := request.Name

	log.Printf("Get dataset status: %s\n", name)

	set, ok := this.sets.Get(name)
	if !ok {
		return nil, errors.New(bbproto.StatusCode_ERROR_NO_DATASET.String())
	}

	return set.dataset, nil

}

//
//
// RECORD API
//
//

func (this *BigBaggerServer) Execute(context context.Context, request *bbproto.RecordRequest) (response *bbproto.RecordResponse, err error) {

	log.Printf("Execute record dataset\n")

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
