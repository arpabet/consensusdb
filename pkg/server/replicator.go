/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import "go.arpabet.com/consensusdb/pkg/pb"

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
}
