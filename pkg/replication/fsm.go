/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"io"
	"sync"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/consensusdb/pkg/ledger"
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

Apply also folds each committed entry into a deterministic hash chain (see
pkg/ledger): identical inputs on every replica ⇒ identical chain, so a divergent
head is proof of corruption/tampering. The head is persisted so it survives
snapshots and restarts, and is exposed for checkpoint signing.
*/
type FSM struct {
	Storage server.KeyValueStorage `inject:""`
	Log     *zap.Logger            `inject:""`

	chainMu sync.Mutex
	chain   *ledger.HashChain
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

func (t *FSM) PostConstruct() error {
	index, digest := ledger.LoadHead(t.Storage)
	t.chain = ledger.NewHashChain(index, digest)
	return nil
}

// ChainHead returns the current hash-chain height and digest (concurrent-safe).
func (t *FSM) ChainHead() (uint64, [32]byte) {
	t.chainMu.Lock()
	defer t.chainMu.Unlock()
	t.ensureChain()
	return t.chain.Head()
}

// ensureChain lazily seeds the chain from the persisted head, so an FSM used
// without PostConstruct (e.g. in tests) still works. Caller holds chainMu.
func (t *FSM) ensureChain() {
	if t.chain == nil {
		index, digest := ledger.LoadHead(t.Storage)
		t.chain = ledger.NewHashChain(index, digest)
	}
}

func (t *FSM) Apply(entry *raft.Log) interface{} {
	// Fold every committed entry into the hash chain first — this is deterministic
	// and covers even entries whose storage op errors, so the chain reflects the
	// exact committed log on every replica. The head is persisted for snapshots.
	t.advanceChain(entry)

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

// advanceChain folds one entry into the hash chain and persists the new head.
func (t *FSM) advanceChain(entry *raft.Log) {
	t.chainMu.Lock()
	t.ensureChain()
	t.chain.Advance(entry.Index, entry.Term, entry.Data)
	index, digest := t.chain.Head()
	t.chainMu.Unlock()
	ledger.StoreHead(t.Storage, index, digest, entry.Index)
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
