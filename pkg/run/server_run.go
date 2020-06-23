/*
 *
 * Copyright 2020-present Arpabet Inc.
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

package run

import (
	"fmt"
	"github.com/consensusdb/consensusdb/pkg/constants"
	"github.com/consensusdb/consensusdb/pkg/swagger"
	"github.com/consensusdb/consensusdb/pkg/pb"
	srv "github.com/consensusdb/consensusdb/pkg/server"
	"github.com/consensusdb/consensusdb/pkg/util"
	rt "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
)


func NewLogger(logdir, filename string) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{
		filename,
	}
	return cfg.Build()
}

func ServerRun() error {

	yamlFile := constants.GetConfigFile()

	fmt.Printf("Load configuration from %s\n", yamlFile)

	conf, err := srv.LoadConfiguration(yamlFile)
	if err != nil {
		return err
	}

	runtime.GOMAXPROCS(conf.NumCPU)

	server, err := srv.NewServer(conf)
	if err != nil {
		return err
	}

	fmt.Printf("GRPC Address: %s\n", conf.GrpcAddress)
	go server.ServeGRPC()

	fmt.Printf("HTTP Address: %s\n", conf.HttpAddress)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpServer, err := NewHttpServer(ctx, conf)
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

	fmt.Println("Server is ready.")

	err = httpServer.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}

var welcomeTpl = util.MustAssetTemplate("templates/welcome.tmpl")

func serveWelcome(w http.ResponseWriter, r *http.Request) {
	welcomeTpl.Execute(w, r)
}

func serveSwagger(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/swagger/")
	p = path.Join("swagger/", p)
	http.ServeFile(w, r, p)
}

func NewHttpServer(ctx context.Context, conf *srv.Configuration) (*http.Server, error) {

	mux := http.NewServeMux()

	gwKeyValue := rt.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := pb.RegisterKeyValueServiceHandlerFromEndpoint(ctx, gwKeyValue, conf.GrpcAddress, opts)
	if err != nil {
		return nil, err
	}
	mux.Handle("/v1/kv", gwKeyValue)
	mux.Handle("/swagger/", util.GzipHandler(http.FileServer(swagger.AssetFile())))
	//mux.HandleFunc("/swagger/", serveSwagger)
	mux.HandleFunc("/", serveWelcome)
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{Addr: conf.HttpAddress, Handler: mux}, nil

}
