#!/usr/bin/env bash
#
# Copyright (c) 2025 Karagatan LLC.
# SPDX-License-Identifier: BUSL-1.1
#
# Regenerates Go protobuf, gRPC, grpc-gateway (v2), and OpenAPI v2 sources
# from proto/cdb.proto.
#
# Requires protoc on PATH and the plugins installed into $(go env GOPATH)/bin:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#   go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
#   go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
#
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

# grpc-gateway v2 module bundles protoc-gen-openapiv2/options/*.proto
GW_VERSION=$(go list -m -f '{{.Version}}' github.com/grpc-ecosystem/grpc-gateway/v2)
GW_DIR="$(go env GOMODCACHE)/github.com/grpc-ecosystem/grpc-gateway/v2@${GW_VERSION}"

protoc \
  -I proto \
  -I third_party \
  -I "${GW_DIR}" \
  --go_out=pkg/pb           --go_opt=paths=source_relative \
  --go-grpc_out=pkg/pb      --go-grpc_opt=paths=source_relative \
  --grpc-gateway_out=pkg/pb --grpc-gateway_opt=paths=source_relative \
  --openapiv2_out=swagger \
  proto/cdb.proto

echo "generated: pkg/pb/cdb.pb.go pkg/pb/cdb_grpc.pb.go pkg/pb/cdb.pb.gw.go swagger/cdb.swagger.json"
