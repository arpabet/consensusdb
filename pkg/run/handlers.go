/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package run

import (
	"context"
	"net/http"
	"strings"
	"text/template"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/swagger"
	"go.arpabet.com/consensusdb/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

/*
GatewayHandler is a servion.HttpHandler that exposes the KeyValueService over
REST/JSON via grpc-gateway. It dials the in-process gRPC server and forwards all
/v1/* requests to it.
*/
type GatewayHandler struct {
	GrpcAddress string      `value:"grpc-server.bind-address,default=127.0.0.1:8442"`
	Log         *zap.Logger `inject:""`
	mux         *runtime.ServeMux
}

func (t *GatewayHandler) PostConstruct() error {
	t.mux = runtime.NewServeMux()
	// the gRPC server may bind 0.0.0.0; dial it over loopback
	dial := strings.Replace(t.GrpcAddress, "0.0.0.0:", "127.0.0.1:", 1)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	t.Log.Info("GatewayHandler", zap.String("dial", dial))
	return pb.RegisterKeyValueServiceHandlerFromEndpoint(context.Background(), t.mux, dial, opts)
}

// Pattern uses a gorilla-mux catch-all so every /v1/* path reaches the gateway.
func (t *GatewayHandler) Pattern() string { return "/v1/{rest:.*}" }

func (t *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mux.ServeHTTP(w, r)
}

/*
SwaggerHandler serves the embedded Swagger UI assets. servion's gzip middleware
compresses the responses automatically.
*/
type SwaggerHandler struct {
	fs http.Handler
}

func (t *SwaggerHandler) PostConstruct() error {
	t.fs = http.FileServer(swagger.AssetFile())
	return nil
}

func (t *SwaggerHandler) Pattern() string { return "/swagger/{rest:.*}" }

func (t *SwaggerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.fs.ServeHTTP(w, r)
}

/*
WelcomeHandler renders the welcome page at the root path. It is the catch-all
handler; more specific patterns (/v1/, /swagger/, /metrics, /healthz) win.
*/
type WelcomeHandler struct {
	tpl *template.Template
}

func (t *WelcomeHandler) PostConstruct() error {
	t.tpl = util.MustAssetTemplate("templates/welcome.tmpl")
	return nil
}

func (t *WelcomeHandler) Pattern() string { return "/" }

func (t *WelcomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.tpl.Execute(w, r)
}
