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
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/consensusdb/consensusdb/cserver"
	"os/signal"
	"flag"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"math/rand"
	"time"
)

var (
	iniFile = flag.String("ini", "consensus.ini", "ini file for initialization")
)

func run() error {

	log.Println("Starting...")

	rand.Seed(time.Now().UnixNano())

	cfg, err := ini.Load(*iniFile)
	if err != nil {
		return err
	}

	curdir, _ := os.Getwd()
	log.Println("Current dir: " + curdir)

	log.Println("Loaded configuration from: " + *iniFile)

	conf, err := cserver.LoadConfiguration(cfg)
	if err != nil {
		return err
	}

	server, err := cserver.NewServer(conf)
	defer server.Close()

	if err != nil {
		return err
	}

	log.Println("GRPC Address: " + conf.GrpcAddress)
	go server.ServeGRPC()

	log.Println("HTTP Address: " + conf.HttpAddress)
	go server.RaftLoop()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpServer, err := NewHttpServer(ctx, conf.HttpAddress, conf.GrpcAddress, server.GetRaftMux())
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

	log.Println("consensusdb is ready.")

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

func NewHttpServer(ctx context.Context, httpAddress, grpcAddress string, mux *http.ServeMux) (*http.Server, error) {

	gwRegion := runtime.NewServeMux()
	gwTnx := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := cserverpb.RegisterRegionServiceHandlerFromEndpoint(ctx, gwRegion, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	err = cserverpb.RegisterTransactionServiceHandlerFromEndpoint(ctx, gwTnx, "localhost"+grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	mux.Handle("/v1/region", gwRegion)
	mux.Handle("/v1/transaction", gwTnx)
	mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{Addr: httpAddress, Handler: mux}, nil

}
