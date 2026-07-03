/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import "context"

/*
Reclaimer physically removes entries whose TTL has elapsed and emits a
WatchDelete for each — the deterministic, raft-driven analogue of store.Sweepable
for the replicated engine.

Expired entries are only hidden on read (lazy expiry); they stay on disk until a
Reclaimer removes them. Reclamation must be deterministic across replicas, so it
runs through the raft log rather than each node deleting on its own wall clock:
only the leader discovers expired keys (a wall-clock scan) and proposes their
removal; every node applies the removal, deleting a key solely when its stored
envelope version is unchanged since discovery.

The concrete implementation lives in pkg/replication; it is defined here as an
interface so the server package does not depend on the replication package.
*/
type Reclaimer interface {

	// ReclaimExpired discovers and removes expired entries, returning how many
	// were removed. On the replicated path it is a no-op on followers (the leader
	// drives it); with replication off it removes directly from local storage.
	ReclaimExpired(ctx context.Context) (removed int, err error)
}
