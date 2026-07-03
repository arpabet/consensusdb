/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"time"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

/*
Reclaimer is the deterministic expiry sweep (server.Reclaimer). A background loop
periodically discovers expired entries and removes them, emitting a WatchDelete
for each.

Determinism: when replication is enabled only the leader discovers expired keys
(a wall-clock scan) and proposes a Reclaim command through raft; every node then
applies the same version-conditioned deletes, so no node ever removes a key based
on its own clock. With replication disabled it reclaims directly from local
storage. Followers do nothing until the leader's Reclaim entries reach them.
*/
type Reclaimer struct {
	Storage    server.KeyValueStorage `inject:""`
	Replicator *Replicator            `inject:""`
	Log        *zap.Logger            `inject:""`

	Interval time.Duration `value:"reclaim.interval,default=30s"`
	Limit    int           `value:"reclaim.batch-size,default=1024"`

	cancel context.CancelFunc
	done   chan struct{}
}

var _ server.Reclaimer = (*Reclaimer)(nil)

func (t *Reclaimer) BeanName() string { return "raft-reclaimer" }

func (t *Reclaimer) PostConstruct() error {
	if t.Interval <= 0 {
		t.Log.Info("ReclaimerDisabled", zap.String("reason", "reclaim.interval <= 0"))
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.done = make(chan struct{})
	go t.loop(ctx)
	return nil
}

func (t *Reclaimer) Destroy() error {
	if t.cancel != nil {
		t.cancel()
		<-t.done
	}
	return nil
}

func (t *Reclaimer) loop(ctx context.Context) {
	defer close(t.done)
	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := t.ReclaimExpired(ctx); err != nil {
				t.Log.Warn("ReclaimExpired", zap.Error(err))
			} else if n > 0 {
				t.Log.Info("Reclaimed", zap.Int("count", n))
			}
		}
	}
}

// ReclaimExpired implements server.Reclaimer.
func (t *Reclaimer) ReclaimExpired(ctx context.Context) (int, error) {

	replicated := t.Replicator != nil && t.Replicator.Enabled()

	// On the replicated path only the leader discovers and proposes; followers
	// pick up the removals when the Reclaim log entries are applied.
	if replicated && !t.Replicator.IsLeader() {
		return 0, nil
	}

	entries, err := t.Storage.ScanExpired(ctx, t.Limit)
	if err != nil || len(entries) == 0 {
		return 0, err
	}

	req := &pb.ReclaimRequest{Entries: entries}
	if replicated {
		return t.Replicator.Reclaim(req) // committed through raft, applied on every node
	}
	return t.Storage.Reclaim(req) // raft-off: remove directly
}
