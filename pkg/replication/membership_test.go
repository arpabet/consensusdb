/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

type testNode struct {
	id        raft.ServerID
	addr      raft.ServerAddress
	transport *raft.InmemTransport
	storage   server.KeyValueStorage
	raft      *raft.Raft
}

func newTestNode(t *testing.T, i int) *testNode {
	t.Helper()
	kv := newStorage(t)
	addr, transport := raft.NewInmemTransport("")
	store := raft.NewInmemStore()
	snaps := raft.NewInmemSnapshotStore()
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(fmt.Sprintf("node-%d", i))
	config.Logger = nil
	// Faster timers so a 3-node cluster elects and replicates quickly in tests.
	config.HeartbeatTimeout = 50 * time.Millisecond
	config.ElectionTimeout = 50 * time.Millisecond
	config.LeaderLeaseTimeout = 50 * time.Millisecond
	config.CommitTimeout = 5 * time.Millisecond

	r, err := raft.NewRaft(config, &FSM{Storage: kv, Log: zap.NewNop()}, store, store, snaps, transport)
	if err != nil {
		t.Fatalf("new raft %d: %v", i, err)
	}
	t.Cleanup(func() { _ = r.Shutdown() })
	return &testNode{id: config.LocalID, addr: addr, transport: transport, storage: kv, raft: r}
}

func waitReplicated(t *testing.T, kv server.KeyValueStorage, key *pb.Key, want string, node int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if rec, err := kv.Get(&pb.KeyRequest{Key: key}); err == nil && rec.Head != nil && string(rec.Value) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("node %d did not replicate %q in time", node, want)
}

// A cluster is formed by bootstrapping a seed and adding voters (what the Join RPC
// does leader-side), and a write on the leader replicates to every follower — the
// full membership + replication path the control plane drives.
func TestMultiNodeMembershipAndReplication(t *testing.T) {
	nodes := []*testNode{newTestNode(t, 0), newTestNode(t, 1), newTestNode(t, 2)}

	// Fully connect the in-memory transports so nodes can reach each other.
	for i := range nodes {
		for j := range nodes {
			if i != j {
				nodes[i].transport.Connect(nodes[j].addr, nodes[j].transport)
			}
		}
	}

	// Seed node bootstraps a single-voter cluster (raft.bootstrap=true equivalent).
	seed := nodes[0]
	if err := seed.raft.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{Suffrage: raft.Voter, ID: seed.id, Address: seed.addr}},
	}).Error(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	waitLeader(t, seed.raft)

	// Leader adds the other two as voters — the AddVoter the Join RPC performs.
	for i := 1; i < len(nodes); i++ {
		if err := seed.raft.AddVoter(nodes[i].id, nodes[i].addr, 0, 0).Error(); err != nil {
			t.Fatalf("add voter %d: %v", i, err)
		}
	}

	// The configuration now lists all three servers.
	cfg := seed.raft.GetConfiguration()
	if err := cfg.Error(); err != nil {
		t.Fatalf("get configuration: %v", err)
	}
	if got := len(cfg.Configuration().Servers); got != 3 {
		t.Fatalf("cluster size = %d, want 3", got)
	}

	// A write committed on the leader replicates to every node's storage.
	key := sampleKey()
	putData, _ := encodeCommand(opPut, &pb.RecordRequest{Key: key, Value: []byte("clustered")})
	if err := seed.raft.Apply(putData, 5*time.Second).Error(); err != nil {
		t.Fatalf("apply on leader: %v", err)
	}
	for i, n := range nodes {
		waitReplicated(t, n.storage, key, "clustered", i)
	}
}
