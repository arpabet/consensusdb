/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

/*
fsmSnapshot streams a full badger backup of the storage engine into the raft
snapshot sink. badger's Backup writes a self-describing stream that Load (used in
FSM.Restore) consumes, so we do not need to serialize records ourselves.
*/
type fsmSnapshot struct {
	storage server.KeyValueStorage
	log     *zap.Logger
}

func (t *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := t.storage.Backup(sink); err != nil {
		sink.Cancel()
		return errors.Wrap(err, "backup storage into snapshot sink")
	}
	return sink.Close()
}

func (t *fsmSnapshot) Release() {}
