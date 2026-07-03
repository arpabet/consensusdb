/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"io"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
FSM is the raft finite state machine for the key-value store. It implements
raftapi.RaftService (glue.InitializingBean + raft.FSM). Apply runs on every node
for each committed log entry and mutates the local storage; reads are served
directly from storage and never go through the log.
*/
type FSM struct {
	Storage server.KeyValueStorage `inject:""`
	Log     *zap.Logger            `inject:""`
}

// fsmResult is returned from Apply and surfaced to the proposer via the
// raft ApplyFuture.Response().
type fsmResult struct {
	status    *pb.Status
	incr      *pb.IncrementResponse // set only for opIncrement
	reclaimed int                   // set only for opReclaim
	err       error
}

func (t *FSM) BeanName() string { return "raft-fsm" }

func (t *FSM) PostConstruct() error { return nil }

func (t *FSM) Apply(entry *raft.Log) interface{} {
	op, msg, err := decodeCommand(entry.Data)
	if err != nil {
		t.Log.Error("FSMDecode", zap.Uint64("index", entry.Index), zap.Error(err))
		return &fsmResult{err: err}
	}
	switch op {
	case opPut:
		// entry.Index is the replica-independent version stamped into the
		// value envelope, identical on every node applying this log entry.
		status, err := t.Storage.Put(msg.(*pb.RecordRequest), entry.Index)
		return &fsmResult{status: status, err: err}
	case opTouch:
		status, err := t.Storage.Touch(msg.(*pb.RecordRequest), entry.Index)
		return &fsmResult{status: status, err: err}
	case opRemove:
		status, err := t.Storage.Remove(msg.(*pb.KeyRequest))
		return &fsmResult{status: status, err: err}
	case opIncrement:
		incr, err := t.Storage.Increment(msg.(*pb.IncrementRequest), entry.Index)
		return &fsmResult{incr: incr, err: err}
	case opBatch:
		status, err := t.Storage.SetBatch(msg.(*pb.BatchRequest), entry.Index)
		return &fsmResult{status: status, err: err}
	case opReclaim:
		// Version-conditioned deletes: deterministic on every replica because the
		// decision uses stored envelope versions, not wall-clock expiry.
		n, err := t.Storage.Reclaim(msg.(*pb.ReclaimRequest))
		return &fsmResult{reclaimed: n, err: err}
	default:
		return &fsmResult{err: xerrors.Errorf("unhandled raft op %d", op)}
	}
}

// Snapshot captures a consistent backup of the storage engine. The backup is
// streamed lazily during Persist so we keep a reference to the storage only.
func (t *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &fsmSnapshot{storage: t.Storage, log: t.Log}, nil
}

// Restore replaces the entire storage contents with the snapshot stream.
func (t *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	if err := t.Storage.Load(rc); err != nil {
		return errors.Wrap(err, "restore snapshot into storage")
	}
	return nil
}
