/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"errors"
	"fmt"

	"go.arpabet.com/consensusdb/pkg/pb"
)

/*
NotLeaderError is returned by write operations when this node is not the raft
leader. It carries the current leader's identity so a client can redirect the
write to the leader.

Writes are redirected client-side (the client re-issues to the leader) rather than
forwarded server-side, on purpose: the raft control-plane ApplyCommand returns only
a Status, so forwarding through it would flatten typed responses (e.g. Increment's
previous/current/version). Redirecting lets the leader return the full typed
response directly.
*/
type NotLeaderError struct {
	LeaderID   string
	LeaderAddr string
}

func (e *NotLeaderError) Error() string {
	if e.LeaderAddr == "" {
		return "not leader: no leader currently elected"
	}
	return fmt.Sprintf("not leader: current leader %q at %q", e.LeaderID, e.LeaderAddr)
}

// AsNotLeader reports whether err is (or wraps) a NotLeaderError and returns it.
func AsNotLeader(err error) (*NotLeaderError, bool) {
	var nl *NotLeaderError
	if errors.As(err, &nl) {
		return nl, true
	}
	return nil, false
}

/*
Replicator is the optional write path. When raft replication is enabled, the
KeyValueService routes mutating operations (Put/Touch/Remove) through the
Replicator so they are committed to the raft log and applied on every node.
When no Replicator bean is present, or Enabled() reports false, the service
falls back to writing directly to local storage (single-node mode).

The concrete implementation lives in pkg/replication; it is defined here as an
interface so the server package does not depend on the replication package
(keeping the dependency one-way: replication -> server).
*/
type Replicator interface {

	// Enabled reports whether replication is active (raft initialized).
	Enabled() bool

	Put(recordRequest *pb.RecordRequest) (*pb.Status, error)

	Touch(recordRequest *pb.RecordRequest) (*pb.Status, error)

	Remove(keyRequest *pb.KeyRequest) (*pb.Status, error)

	Increment(req *pb.IncrementRequest) (*pb.IncrementResponse, error)

	Batch(req *pb.BatchRequest) (*pb.Status, error)
}
