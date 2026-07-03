/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// This is the mechanism staphi's Broker relies on once it swaps in-process pubsub
// for store.Watch: a value written through ONE cdb client (replica B) is delivered
// to a prefix watcher on ANOTHER cdb client (replica A) over the wire. That cross-
// client delivery — fed by the consensusdb apply path — is what lets a browser
// watching a job on one stateless replica see applications written on another.
func TestCdbProviderCrossReplicaWatch(t *testing.T) {
	tmp := t.TempDir()
	probe := &vrpcDataProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.VrpcDataService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)
	glueCtx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer glueCtx.Close()
	addr := "tcp://" + probe.Server.Addr().String()

	// Two replicas sharing one cluster store.
	watcher, err := cdb.New("replica-A", addr, "", "STAPHI")
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	defer watcher.Destroy()
	writer, err := cdb.New("replica-B", addr, "", "STAPHI")
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	defer writer.Destroy()

	// Watch the job's application prefix (staphi keys apps "app/<jobId>/<appId>").
	prefix := []byte("app/job-1/")
	got := make(chan []byte, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = watcher.Watch(ctx).ByRawPrefix(prefix).Do(func(ev *store.WatchEvent) bool {
			if ev.Type == store.WatchSet {
				b := make([]byte, len(ev.Value))
				copy(b, ev.Value)
				got <- b
			}
			return true
		})
	}()
	time.Sleep(200 * time.Millisecond) // let the server-side watch subscribe

	// A non-matching key must not reach this watcher.
	if err := writer.SetRaw(ctx, []byte("app/job-2/x"), []byte("{}"), store.NoTTL); err != nil {
		t.Fatalf("set other job: %v", err)
	}

	// Replica B registers an applicant on the shared store.
	type applicant struct {
		Id    string `json:"id"`
		JobId string `json:"jobId"`
	}
	payload, _ := json.Marshal(applicant{Id: "a1", JobId: "job-1"})
	if err := writer.SetRaw(ctx, []byte("app/job-1/a1"), payload, store.NoTTL); err != nil {
		t.Fatalf("set applicant: %v", err)
	}

	select {
	case v := <-got:
		// WatchEvent.Value is the plain value (version/ttl are separate fields),
		// so the watcher decodes it directly — exactly as the Broker does.
		var a applicant
		if err := json.Unmarshal(v, &a); err != nil {
			t.Fatalf("decode watched value %q: %v", v, err)
		}
		if a.Id != "a1" || a.JobId != "job-1" {
			t.Fatalf("watched applicant = %+v, want a1/job-1", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher on replica A never saw the write from replica B")
	}

	// And only the matching key was delivered.
	select {
	case v := <-got:
		t.Fatalf("unexpected extra event: %q", v)
	case <-time.After(200 * time.Millisecond):
	}
}
