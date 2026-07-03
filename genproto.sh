#!/usr/bin/env bash
#
# Copyright (c) 2025 Karagatan LLC.
# SPDX-License-Identifier: BUSL-1.1
#
# Regenerates the Go protobuf message types from proto/cdb.proto.
# The API is served over value-rpc (pkg/server/vrpc_data.go) — there is no gRPC
# service, gateway, or OpenAPI generation.
#
# Requires protoc on PATH and the plugin installed into $(go env GOPATH)/bin:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

protoc \
  -I proto \
  --go_out=pkg/pb --go_opt=paths=source_relative \
  proto/cdb.proto

echo "generated: pkg/pb/cdb.pb.go"
