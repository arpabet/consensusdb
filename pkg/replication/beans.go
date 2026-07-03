/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"go.arpabet.com/raft/raftmod"
	"go.arpabet.com/raft/raftvrpc"
)

/*
Beans returns the replication bean set to register in the run scope alongside the
storage and gRPC service. It bundles:

  - the sprint bridge beans (Application, NodeService, env resolver) and the
    hclog factory that raftmod requires for dependency injection;
  - the "raft-store" badger managed data store backing the raft log;
  - the raftmod stores/lookup/server beans needed to run a raft node (the serf
    membership beans are intentionally omitted — see RaftHost);
  - the FSM, the Replicator (server.Replicator write path) and the RaftHost that
    drives the raft server lifecycle.

Replication stays dormant until raft.bind-address and serf.bind-address are set;
without them KeyValueService writes go straight to local storage.
*/
func Beans() []interface{} {
	return []interface{}{
		NewApplication(),
		NewNodeService(),
		NewEnvPropertyResolver(),
		HCLogFactory(),

		RaftStoreFactory(),

		raftmod.RaftLogStoreFactory(),
		raftmod.RaftStableStoreFactory(),
		raftmod.RaftSnapshotFactory(),
		raftmod.ServerLookup(),
		raftmod.RaftServer(),

		// value-rpc control plane: a hosted vrpc server, the raftvrpc control
		// service (Bootstrap/Join/GetConfiguration/ApplyCommand) registered on it,
		// and the client pool used to reach the leader (forwarding + membership).
		VrpcServerFactory("vrpc-server"),
		raftvrpc.RaftVrpcClientPool(),
		raftvrpc.RaftVrpcServer(),

		&FSM{},
		&Replicator{},
		&Reclaimer{},
		&RaftHost{},
	}
}
