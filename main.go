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

package main

import (
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
	"flag"
)

var (
	iniFile = flag.String("ini", "bigbagger.ini", "ini file for initialization")
)

func run() error {

	log.Println("BigBagger DataNode started from " + *iniFile)

	cfg, err := ini.Load(*iniFile)
	if err != nil {
		return err
	}

	httpAddress := cfg.Section("server").Key("httpAddress").String()
	grpcAddress := cfg.Section("server").Key("grpcAddress").String()
	dataDir := cfg.Section("database").Key("dataDir").String()

	security, err := bbserver.NewSimpleSecurityContext(cfg.Section("security").KeysHash())

	if err != nil {
		return err
	}

	server, err := bbserver.NewServer(dataDir, security)
	defer server.Close()

	if err != nil {
		return err
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
		return err
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

	err = httpServer.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

var welcomeTpl = template.Must(template.ParseFiles("templates/welcome.tmpl"))

func serveWelcome(w http.ResponseWriter, r *http.Request) {
	welcomeTpl.Execute(w, r)
}

func serveSwagger(w http.ResponseWriter, r *http.Request) {
	//swagger := http.FileServer(http.Dir("./3rdparty/swagger-ui"))
	log.Println("request", r.URL.Path)
	p := strings.TrimPrefix(r.URL.Path, "/swagger/")
	p = path.Join("3rdparty/swagger-ui/", p)
	log.Println("request map ", p)
	http.ServeFile(w, r, p)

}

func NewHttpServer(ctx context.Context, httpAddress, grpcAddress string) (*http.Server, error) {

	gwTable := runtime.NewServeMux()
	gwTnx := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := bbproto.RegisterTableServiceHandlerFromEndpoint(ctx, gwTable, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	err = bbproto.RegisterTransactionServiceHandlerFromEndpoint(ctx, gwTnx, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/table", gwTable)
	mux.Handle("/v1/transaction", gwTnx)
	mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)

	curdir, _ := os.Getwd()
	log.Println("cur dir", curdir)

	return &http.Server{Addr: httpAddress, Handler: mux}, nil

}
