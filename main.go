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
	"encoding/json"
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

	server, err := bbserver.NewServer(cfg)
	defer server.Close();

	if err != nil {
		log.Fatal("fail to create a bbserver ", err)
		os.Exit(1)
	}

	log.Println("Starting gRPC bbserver on " + grpcAddress)
	go server.StartServer(grpcAddress)

	log.Println("Starting HTTP bbserver on " + httpAddress)
	httpServer, err := NewHttpServer(httpAddress, grpcAddress)
	if err != nil {
		log.Fatal("port is busy " + httpAddress, err)
	}

	signalChain := make(chan os.Signal, 1)
	signal.Notify(signalChain, os.Interrupt)
	go func(){
		for _ = range signalChain {
			server.Close()
			httpServer.Close()
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

	client, err := bbclient.NewClient(grpcAddress, "ahahahahaha")
	defer client.Close()

	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile("create.json")
	if err != nil {
		return err
	}

	var dataset bbproto.Dataset

	json.Unmarshal(data, &dataset)

	err = client.CreateDataset(&dataset)

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

func NewHttpServer(httpAddress, grpcAddress string) (*http.Server, error) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	gw := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := bbproto.RegisterDatasetServiceHandlerFromEndpoint(ctx, gw, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	err = bbproto.RegisterRecordServiceHandlerFromEndpoint(ctx, gw, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)
	curdir, _ := os.Getwd()
	fmt.Println("cur dir", curdir)
	//swagger := http.FileServer(http.Dir(filepath.Join(curdir, "3rdparty", "swagger-ui")))
	//mux.Handle("/swagger/", swagger)
	mux.Handle("/v1/", gw)

	return &http.Server{Addr: httpAddress, Handler: mux}, nil

}
