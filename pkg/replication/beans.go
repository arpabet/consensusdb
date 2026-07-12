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
BaseBeans are the replication-package beans a node always needs, in single-node
mode as well as cluster mode:

  - the sprint bridge beans (Application, NodeService, env resolver) and the
    hclog factory that raftmod requires for dependency injection;
  - the value-rpc data-plane host (the vrpc server that serves the key-value
    operations to clients). It is a mem/dormant server when
    vrpc-server.bind-address is empty, so it never fails to construct.

These carry no raft dependency, so a single-node process constructs cleanly with
only these; KeyValueService writes go straight to local storage.
*/
func BaseBeans() []interface{} {
	return []interface{}{
		NewApplication(),
		NewNodeService(),
		NewEnvPropertyResolver(),
		HCLogFactory(),

		// value-rpc data-plane host for client key-value operations.
		VrpcServerFactory("vrpc-server"),
	}
}

/*
ClusterBeans are the raft/replication beans a node needs only when it
participates in a cluster. They are registered only in cluster mode (see
config.ResolveMode); in single-node mode they are omitted entirely, so the
raftmod RaftServer — which servion would otherwise drive to Serve an unbound
transport and panic — is simply not present. The bundle is:

  - the "raft-store" badger managed data store backing the raft log;
  - the raftmod stores/lookup/server beans needed to run a raft node (the serf
    membership beans are intentionally omitted — see RaftHost);
  - the raftvrpc control plane (Bootstrap/Join/GetConfiguration/ApplyCommand) and
    the client pool used to reach the leader (forwarding + membership);
  - the FSM, the Replicator (server.Replicator write path), the Reclaimer and the
    RaftHost that drives the raft server lifecycle;
  - the AddressReconciler that re-registers a node whose recorded membership
    address drifted from consensusdb.advertise-address (seed genesis records the
    private IP; Kubernetes reschedules change it);
  - the verifiable-ledger LedgerService (the ledger.digest control function, S6),
    which co-signs the FSM's hash chain and therefore only exists with raft.

Every required (inject:"") reference to these types lives inside this bundle
(they inject one another); everything outside — server.*, console.* — injects
them as inject:"optional", so omitting the bundle leaves no dependency unmet.
*/
func ClusterBeans() []interface{} {
	return []interface{}{
		RaftStoreFactory(),

		// Mandatory node↔node mutual TLS on the consensus transport: this factory
		// provisions the node identity (seed genesis or join enrollment) and yields
		// the "raft-transport-tls" config raftmod's RaftServer injects.
		NodeTLSConfigFactory(),

		raftmod.RaftLogStoreFactory(),
		raftmod.RaftStableStoreFactory(),
		raftmod.RaftSnapshotFactory(),
		raftmod.ServerLookup(),
		raftmod.RaftServer(),

		raftvrpc.RaftVrpcClientPool(),
		raftvrpc.RaftVrpcServer(),

		&FSM{},
		&Replicator{},
		&Reclaimer{},
		&RaftHost{},

		// Keeps this node's raft membership record equal to its advertise
		// address (heals the seed's genesis-time IP and any reschedule drift).
		&AddressReconciler{},

		&LedgerService{},
	}
}

/*
Beans returns the full replication bean set (base + cluster). It is retained for
callers and tests that want the whole stack regardless of mode; the run wiring in
main.go registers BaseBeans always and ClusterBeans only in cluster mode.
*/
func Beans() []interface{} {
	return append(BaseBeans(), ClusterBeans()...)
}
