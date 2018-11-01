package bbserver

import (
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/ini.v1"
	"log"
	"net"
	"bigbagger/proto/bbproto"
)

type BigBaggerServer struct {
	grpcServer   *grpc.Server
	dataDir string
}

func NewServer(cfg *ini.File) (server *BigBaggerServer, err error) {

	server = new(BigBaggerServer)

	section := cfg.Section("database")

	server.dataDir = section.Key("dataDir").String()

	fmt.Print("init dataDir=", server.dataDir, "\n")

	return server, nil
}


func (this *BigBaggerServer) Create(context context.Context, request *bbproto.CreateDatasetRequest) (response *bbproto.CreateDatasetResponse, err error) {

	log.Printf("Create dataset: %s\n", request.Dataset.Name)

	response = new(bbproto.CreateDatasetResponse)
	response.Name = request.Dataset.Name

	return response, nil

}

func (this *BigBaggerServer) Update(context context.Context, request *bbproto.UpdateDatasetRequest) (response *bbproto.UpdateDatasetResponse, err error) {

	log.Printf("Update dataset: %s\n", request.Dataset.Name)

	response = new(bbproto.UpdateDatasetResponse)
	response.Name = request.Dataset.Name

	return response, nil

}

func (this *BigBaggerServer) Delete(context context.Context, request *bbproto.DeleteDatasetRequest) (response *bbproto.DeleteDatasetResponse, err error) {

	log.Printf("Delete dataset: %s\n", request.Name)

	response = new(bbproto.DeleteDatasetResponse)
	response.Name = request.Name

	return response, nil
}

func (this *BigBaggerServer) List(context context.Context, request *bbproto.ListDatasetsRequest) (response *bbproto.ListDatasetsResponse, err error) {

	log.Printf("List datasets\n")

	response = new(bbproto.ListDatasetsResponse)

	return response, nil

}

func (this *BigBaggerServer) Status(context context.Context, request *bbproto.GetDatasetStatusRequest) (response *bbproto.GetDatasetStatusResponse, err error) {

	log.Printf("Get dataset status: %s\n", request.Name)

	response = new(bbproto.GetDatasetStatusResponse)

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
	// Register service
	bbproto.RegisterBigBaggerServiceServer(this.grpcServer, this)
	// Start serving requests
	return this.grpcServer.Serve(listen)

}

func (this *BigBaggerServer) Stop() {
	if this.grpcServer != nil {
		this.grpcServer.Stop()
	}
}
