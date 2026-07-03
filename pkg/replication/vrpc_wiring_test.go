/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication_test

import (
	"context"
	"strings"
	"testing"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/raft/raftapi"
	"go.arpabet.com/raft/raftvrpc"
	"go.arpabet.com/value-rpc/valueclient"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

type vrpcProbe struct {
	VrpcServer valueserver.Server     `inject:""`
	Pool       raftapi.RaftClientPool `inject:""`
}

func (p *vrpcProbe) PostConstruct() error { return nil }

// The value-rpc control plane binds a real TCP endpoint, the raftvrpc control
// service registers on it, and a client can reach it: with raft disabled,
// Bootstrap returns "raft not initialized" — proving the RPC dispatched to the
// handler (not a missing function) end-to-end through the wired bean graph.
func TestVrpcControlPlaneReachable(t *testing.T) {
	tmp := t.TempDir()

	probe := &vrpcProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
			// raft.bind-address unset -> raft disabled
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.KeyValueService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)

	ctx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer ctx.Close()

	if probe.Pool == nil {
		t.Fatal("RaftClientPool not injected")
	}
	if probe.VrpcServer == nil || probe.VrpcServer.Addr() == nil {
		t.Fatal("vrpc server not bound")
	}

	cli := valueclient.NewClient("tcp://"+probe.VrpcServer.Addr().String(), "", valueclient.WithLogger(zap.NewNop()))
	if err := cli.Connect(); err != nil {
		t.Fatalf("connect to control endpoint: %v", err)
	}
	defer cli.Close()

	_, err = raftvrpc.CallBootstrap(context.Background(), cli)
	if err == nil {
		t.Fatal("expected an error with raft disabled")
	}
	if !strings.Contains(err.Error(), "raft not initialized") {
		t.Fatalf("error = %v, want 'raft not initialized' (proves the handler was reached)", err)
	}
}
