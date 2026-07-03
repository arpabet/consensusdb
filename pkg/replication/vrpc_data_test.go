/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/value-rpc/valueclient"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

type vrpcDataProbe struct {
	Server valueserver.Server `inject:""`
}

func (p *vrpcDataProbe) PostConstruct() error { return nil }

func dataKey(minor string) *pb.Key {
	return &pb.Key{MajorKey: []byte("t"), RegionName: []byte("R"), MinorKey: []byte(minor)}
}

// The key-value data plane round-trips over value-rpc through the full wired bean
// graph: put/get, increment, and atomic batch all work against local storage
// (raft disabled here), exercising the codecs and the shared KeyValueService.
func TestVrpcDataPlaneRoundTrip(t *testing.T) {
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

	ctx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer ctx.Close()

	if probe.Server == nil || probe.Server.Addr() == nil {
		t.Fatal("vrpc server not bound")
	}
	cli := valueclient.NewClient("tcp://"+probe.Server.Addr().String(), "", valueclient.WithLogger(zap.NewNop()))
	if err := cli.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cli.Close()
	bg := context.Background()

	// Put then Get.
	k := dataKey("a")
	if st, err := server.CallPut(bg, cli, &pb.RecordRequest{Key: k, Value: []byte("hello")}); err != nil || !st.Updated {
		t.Fatalf("put: st=%+v err=%v", st, err)
	}
	rec, err := server.CallGet(bg, cli, &pb.KeyRequest{Key: k})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Head == nil || string(rec.Value) != "hello" {
		t.Fatalf("get = %+v / %q, want found 'hello'", rec.Head, rec.Value)
	}

	// A missing key comes back not-found (Head nil).
	if miss, err := server.CallGet(bg, cli, &pb.KeyRequest{Key: dataKey("missing")}); err != nil || miss.Head != nil {
		t.Fatalf("get missing = %+v err=%v, want not-found", miss, err)
	}

	// Increment.
	ck := dataKey("counter")
	if r, err := server.CallIncrement(bg, cli, &pb.IncrementRequest{Key: ck, Initial: 10, Delta: 5}); err != nil || r.Previous != 10 || r.Current != 15 {
		t.Fatalf("increment = %+v err=%v, want prev 10 current 15", r, err)
	}

	// Atomic batch, then read both back.
	b1, b2 := dataKey("b1"), dataKey("b2")
	if st, err := server.CallBatch(bg, cli, &pb.BatchRequest{Records: []*pb.RecordRequest{
		{Key: b1, Value: []byte("one")},
		{Key: b2, Value: []byte("two")},
	}}); err != nil || !st.Updated {
		t.Fatalf("batch: st=%+v err=%v", st, err)
	}
	for _, tc := range []struct {
		key  *pb.Key
		want string
	}{{b1, "one"}, {b2, "two"}} {
		if rec, err := server.CallGet(bg, cli, &pb.KeyRequest{Key: tc.key}); err != nil || string(rec.Value) != tc.want {
			t.Fatalf("get %s = %q err=%v, want %q", tc.key.MinorKey, rec.Value, err, tc.want)
		}
	}

	// Remove then confirm gone.
	if st, err := server.CallRemove(bg, cli, &pb.KeyRequest{Key: k}); err != nil || !st.Updated {
		t.Fatalf("remove: st=%+v err=%v", st, err)
	}
	if rec, err := server.CallGet(bg, cli, &pb.KeyRequest{Key: k}); err != nil || rec.Head != nil {
		t.Fatalf("get after remove = %+v err=%v, want not-found", rec, err)
	}

	// Enumerate: three records under a fresh region stream back.
	region := func(minor string) *pb.Key {
		return &pb.Key{MajorKey: []byte("t"), RegionName: []byte("E"), MinorKey: []byte(minor)}
	}
	for _, mk := range []string{"e1", "e2", "e3"} {
		if _, err := server.CallPut(bg, cli, &pb.RecordRequest{Key: region(mk), Value: []byte(mk)}); err != nil {
			t.Fatalf("put %s: %v", mk, err)
		}
	}
	var enumErr error
	recs, err := server.EnumerateStream(bg, cli,
		&pb.EnumerateRequest{Prefix: &pb.Key{MajorKey: []byte("t"), RegionName: []byte("E")}, Ordered: true}, 16, &enumErr)
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	count := 0
	for range recs {
		count++
	}
	if enumErr != nil {
		t.Fatalf("enumerate stream error: %v", enumErr)
	}
	if count != 3 {
		t.Fatalf("enumerate returned %d records, want 3", count)
	}

	// Watch: a put under the watched prefix delivers a Set event.
	wctx, cancel := context.WithCancel(bg)
	defer cancel()
	var watchErr error
	events, err := server.WatchStream(wctx, cli,
		&pb.WatchRequest{Prefix: &pb.Key{MajorKey: []byte("t"), RegionName: []byte("W")}}, 16, &watchErr)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // let the server-side WatchRaw subscribe

	wk := &pb.Key{MajorKey: []byte("t"), RegionName: []byte("W"), MinorKey: []byte("wkey")}
	if _, err := server.CallPut(bg, cli, &pb.RecordRequest{Key: wk, Value: []byte("watched")}); err != nil {
		t.Fatalf("put watched: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Type != pb.ChangeType_WATCH_SET || string(ev.Value) != "watched" {
			t.Fatalf("watch event = %+v, want SET 'watched'", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no watch event delivered")
	}
}
