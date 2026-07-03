/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

// notLeaderRaftServer implements raftapi.RaftServer over a real raft; only Raft()
// is exercised by the Replicator (the rest are no-ops to satisfy the interface).
type notLeaderRaftServer struct{ r *raft.Raft }

func (s *notLeaderRaftServer) Raft() (*raft.Raft, bool)                 { return s.r, s.r != nil }
func (s *notLeaderRaftServer) Transport() (raft.Transport, bool)        { return nil, false }
func (s *notLeaderRaftServer) IsLeader() bool                           { return s.r.State() == raft.Leader }
func (s *notLeaderRaftServer) PostConstruct() error                     { return nil }
func (s *notLeaderRaftServer) Destroy() error                           { return nil }
func (s *notLeaderRaftServer) Bind() error                             { return nil }
func (s *notLeaderRaftServer) Alive() bool                             { return true }
func (s *notLeaderRaftServer) ListenAddress() net.Addr                 { return nil }
func (s *notLeaderRaftServer) Serve() error                            { return nil }
func (s *notLeaderRaftServer) Shutdown() error                         { return nil }
func (s *notLeaderRaftServer) ShutdownCh() <-chan struct{}             { return nil }
func (s *notLeaderRaftServer) BeanName() string                        { return "test-raft-server" }
func (s *notLeaderRaftServer) GetStats(func(string, string) bool) error { return nil }

type noopFSM struct{}

func (noopFSM) Apply(*raft.Log) interface{}        { return nil }
func (noopFSM) Snapshot() (raft.FSMSnapshot, error) { return nil, nil }
func (noopFSM) Restore(io.ReadCloser) error         { return nil }

// A write on a non-leader node is rejected with a structured NotLeaderError so a
// client can redirect it to the leader (which returns the full typed response).
func TestReplicatorRejectsNonLeaderWrite(t *testing.T) {
	// A fresh, un-bootstrapped raft node stays a Follower with no leader.
	store := raft.NewInmemStore()
	snaps := raft.NewInmemSnapshotStore()
	_, transport := raft.NewInmemTransport("")
	config := raft.DefaultConfig()
	config.LocalID = "node-follower"
	config.Logger = nil
	r, err := raft.NewRaft(config, noopFSM{}, store, store, snaps, transport)
	if err != nil {
		t.Fatalf("new raft: %v", err)
	}
	defer r.Shutdown()

	if r.State() == raft.Leader {
		t.Fatal("un-bootstrapped node should not be leader")
	}

	repl := &Replicator{
		RaftServer: &notLeaderRaftServer{r: r},
		Log:        zap.NewNop(),
		Timeout:    2 * time.Second,
	}

	// Every mutating op should reject the same way; spot-check Put and Increment.
	if _, err := repl.Put(&pb.RecordRequest{Key: sampleKey(), Value: []byte("x")}); err == nil {
		t.Fatal("expected NotLeaderError on a non-leader Put")
	} else if nl, ok := server.AsNotLeader(err); !ok {
		t.Fatalf("Put error = %v, want *server.NotLeaderError", err)
	} else if nl.LeaderAddr != "" {
		t.Fatalf("no leader elected yet, want empty LeaderAddr, got %q", nl.LeaderAddr)
	}

	if _, err := repl.Increment(&pb.IncrementRequest{Key: sampleKey(), Delta: 1}); err == nil {
		t.Fatal("expected NotLeaderError on a non-leader Increment")
	} else if _, ok := server.AsNotLeader(err); !ok {
		t.Fatalf("Increment error = %v, want *server.NotLeaderError", err)
	}
}
