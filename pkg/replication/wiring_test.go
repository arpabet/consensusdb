/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication_test

import (
	"bytes"
	"context"
	"testing"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.uber.org/zap"
)

// wiringProbe captures the wired KeyValueService and the (optional) Replicator
// so the test can exercise them after the glue container is built.
type wiringProbe struct {
	Service    *server.KeyValueService `inject:""`
	Replicator server.Replicator       `inject:"optional"`
}

func (p *wiringProbe) PostConstruct() error { return nil }

// TestBeanGraphWires builds the full replication bean graph (the raftmod stores,
// server and lookup, plus the sprint shims) through glue with raft disabled. It
// proves every raftmod injection resolves against the bridge beans, and that the
// write path falls back to local storage when replication is off.
func TestBeanGraphWires(t *testing.T) {
	tmp := t.TempDir()

	probe := &wiringProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir": tmp,
			"application.data.dir": tmp,
			// raft.bind-address / serf.bind-address intentionally unset -> disabled
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

	if probe.Replicator == nil {
		t.Fatal("Replicator bean was not injected")
	}
	if probe.Replicator.Enabled() {
		t.Fatal("replication should be disabled without raft.bind-address")
	}
	if probe.Service == nil {
		t.Fatal("KeyValueService was not injected")
	}

	// With replication disabled, writes go straight to local storage.
	key := &pb.Key{MajorKey: []byte("alex"), RegionName: []byte("accounts"), MinorKey: []byte("Bank1")}
	if _, err := probe.Service.Put(context.Background(), &pb.RecordRequest{Key: key, Value: []byte("wired")}); err != nil {
		t.Fatalf("put: %v", err)
	}
	rec, err := probe.Service.Get(context.Background(), &pb.KeyRequest{Key: key})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(rec.Value, []byte("wired")) {
		t.Fatalf("value = %q, want wired", rec.Value)
	}
}
