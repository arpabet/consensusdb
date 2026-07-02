/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/sprint/raftapi"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
)

/*
Replicator implements server.Replicator. Mutating operations are encoded as raft
commands and committed through raft.Apply on the leader; the FSM then applies
them to local storage on every node. Reads do not go through here.

This node must be the leader to accept writes. Forwarding writes from a follower
to the leader (via raftapi.RaftClientPool / raftgrpc) is not yet implemented, so
in a single-node deployment this is always the leader and works directly.
*/
type Replicator struct {
	RaftServer raftapi.RaftServer `inject:""`
	Log        *zap.Logger        `inject:""`

	Timeout time.Duration `value:"raft.apply-timeout,default=10s"`
}

func (t *Replicator) BeanName() string { return "raft-replicator" }

// Enabled reports whether raft has been initialized (replication is active).
func (t *Replicator) Enabled() bool {
	_, ok := t.RaftServer.Raft()
	return ok
}

func (t *Replicator) Put(recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return t.apply(opPut, recordRequest)
}

func (t *Replicator) Touch(recordRequest *pb.RecordRequest) (*pb.Status, error) {
	return t.apply(opTouch, recordRequest)
}

func (t *Replicator) Remove(keyRequest *pb.KeyRequest) (*pb.Status, error) {
	return t.apply(opRemove, keyRequest)
}

func (t *Replicator) Increment(req *pb.IncrementRequest) (*pb.IncrementResponse, error) {
	res, err := t.applyCommand(opIncrement, req)
	if err != nil {
		return nil, err
	}
	return res.incr, res.err
}

func (t *Replicator) Batch(req *pb.BatchRequest) (*pb.Status, error) {
	return t.apply(opBatch, req)
}

func (t *Replicator) Reclaim(req *pb.ReclaimRequest) (int, error) {
	res, err := t.applyCommand(opReclaim, req)
	if err != nil {
		return 0, err
	}
	return res.reclaimed, res.err
}

// IsLeader reports whether this node is the current raft leader. Used by the
// Reclaimer so only the leader discovers expired keys and proposes their removal.
func (t *Replicator) IsLeader() bool {
	r, ok := t.RaftServer.Raft()
	return ok && r.State() == raft.Leader
}

// applyCommand commits op(msg) through raft on the leader and returns the FSM
// result. Callers pick the field they need (status or incr).
func (t *Replicator) applyCommand(op opCode, msg proto.Message) (*fsmResult, error) {
	r, ok := t.RaftServer.Raft()
	if !ok {
		return nil, xerrors.New("raft not initialized")
	}
	if r.State() != raft.Leader {
		// TODO: forward to the current leader via raftgrpc/RaftClientPool.
		return nil, xerrors.Errorf("not leader, current leader is %q", string(t.leaderAddr(r)))
	}

	data, err := encodeCommand(op, msg)
	if err != nil {
		return nil, err
	}

	future := r.Apply(data, t.Timeout)
	if err := future.Error(); err != nil {
		return nil, errors.Wrap(err, "raft apply")
	}

	res, ok := future.Response().(*fsmResult)
	if !ok {
		return nil, xerrors.Errorf("unexpected fsm response type %T", future.Response())
	}
	return res, nil
}

func (t *Replicator) apply(op opCode, msg proto.Message) (*pb.Status, error) {
	res, err := t.applyCommand(op, msg)
	if err != nil {
		return nil, err
	}
	return res.status, res.err
}

func (t *Replicator) leaderAddr(r *raft.Raft) raft.ServerAddress {
	addr, _ := r.LeaderWithID()
	return addr
}
