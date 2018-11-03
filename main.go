package main

import (
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/ini.v1"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"text/template"
	"bigbagger/proto/bbproto"
	"bigbagger/bbserver"
	"os/signal"
	"bigbagger/bbclient"
	"io/ioutil"
	"github.com/golang/protobuf/jsonpb"
	"time"
)

func main() {

	println("BigBagger DataNode")

	cfg, err := ini.Load("bigbagger.ini")
	if err != nil {
		fmt.Printf("Fail to read ini file: %v", err)
		os.Exit(1)
	}

	httpAddress := cfg.Section("server").Key("httpAddress").String()
	grpcAddress := cfg.Section("server").Key("grpcAddress").String()
	dataDir := cfg.Section("database").Key("dataDir").String()

	server, err := bbserver.NewServer(dataDir)
	defer server.Close()

	if err != nil {
		log.Fatal("fail to create a bbserver ", err)
		os.Exit(1)
	}

	log.Println("Starting gRPC server on " + grpcAddress)
	go server.StartServer(grpcAddress)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Println("Starting HTTP server on " + httpAddress)
	httpServer, err := NewHttpServer(ctx, httpAddress, grpcAddress)
	defer httpServer.Close()

	if err != nil {
		log.Fatal("port is busy " + httpAddress, err)
	}

	signalChain := make(chan os.Signal, 1)
	signal.Notify(signalChain, os.Interrupt)
	go func(){
		for _ = range signalChain {
			httpServer.Close()
			cancel()
			server.Close()
		}
	}()

	// Do some client stuff

	err = testClient(grpcAddress)
	if err != nil {
		log.Fatal("Test client failed: ", err)
	}


	err = httpServer.ListenAndServe()
	if err != nil {
		log.Fatal("Exit: ", err)
	}


	/*
		bbclient, err := bbclient.NewClient(grpcAddress)

		if err != nil {
			fmt.Println("error connecting: ", err)
		}

		err = bbclient.CreateDataset("test")

		if err != nil {
			fmt.Println("error copy: ", err)
		}

		bbclient.Close()
		bbserver.Stop()
	*/

}

func testClient(grpcAddress string) error {

	time.Sleep(time.Second)

	/*

	ds := bbproto.Dataset{}
	ds.Name = "TEST"
	ds.Distr = bbproto.DataDistribution_DD_REPLICATION
	ds.Compression = bbproto.Compression_DC_ZLIB
	ds.Pit = bbproto.PointInTime_PIT_ALL
	ds.TtlSeconds = 86356

	str, err := new(jsonpb.Marshaler).MarshalToString(&ds)
	ioutil.WriteFile("example.json", []byte(str), 0755)

	*/

	client, err := bbclient.NewClient(grpcAddress)
	defer client.Close()

	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile("example.json")
	if err != nil {
		return err
	}

	dataset := new(bbproto.Dataset)
	err = jsonpb.UnmarshalString(string(data), dataset)

	if err != nil {
		return err
	}

	err = client.CreateDataset(dataset)

	return err

}

var welcomeTpl = template.Must(template.ParseFiles("templates/welcome.tmpl"))

func serveWelcome(w http.ResponseWriter, r *http.Request) {
	welcomeTpl.Execute(w, r)
}

func serveSwagger(w http.ResponseWriter, r *http.Request) {
	//swagger := http.FileServer(http.Dir("./3rdparty/swagger-ui"))
	fmt.Println("request", r.URL.Path)
	p := strings.TrimPrefix(r.URL.Path, "/swagger/")
	p = path.Join("3rdparty/swagger-ui/", p)
	fmt.Println("request map ", p)
	http.ServeFile(w, r, p)

}

func NewHttpServer(ctx context.Context, httpAddress, grpcAddress string) (*http.Server, error) {

	gwDataset := runtime.NewServeMux()
	gwTnx := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := bbproto.RegisterDatasetServiceHandlerFromEndpoint(ctx, gwDataset, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	err = bbproto.RegisterTransactionServiceHandlerFromEndpoint(ctx, gwTnx, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/dataset", gwDataset)
	mux.Handle("/v1/transaction", gwTnx)
	mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)

	curdir, _ := os.Getwd()
	fmt.Println("cur dir", curdir)

	return &http.Server{Addr: httpAddress, Handler: mux}, nil

}
