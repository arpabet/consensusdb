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

	if err != nil {
		log.Fatal("fail to create a bbserver ", err)
		os.Exit(1)
	}

	log.Println("Starting gRPC bbserver on " + grpcAddress)
	go server.StartServer(grpcAddress)

	log.Println("Starting HTTP bbserver on " + httpAddress)
	err = run(httpAddress, grpcAddress)
	if err != nil {
		log.Fatal("port is busy " + httpAddress, err)
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

func run(httpAddress, grpcAddress string) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	gw := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := bbproto.RegisterBigBaggerServiceHandlerFromEndpoint(ctx, gw, "localhost"+grpcAddress, opts)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)
	curdir, _ := os.Getwd()
	fmt.Println("cur dir", curdir)
	//swagger := http.FileServer(http.Dir(filepath.Join(curdir, "3rdparty", "swagger-ui")))
	//mux.Handle("/swagger/", swagger)
	mux.Handle("/v1/", gw)

	return http.ListenAndServe(httpAddress, mux)
}
