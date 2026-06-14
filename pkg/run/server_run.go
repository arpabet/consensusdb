/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"fmt"
	"go.arpabet.com/consensusdb/pkg/constants"
	"go.arpabet.com/consensusdb/pkg/swagger"
	"go.arpabet.com/consensusdb/pkg/pb"
	srv "go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/consensusdb/pkg/util"
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
