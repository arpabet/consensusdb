/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"time"

	"github.com/hashicorp/raft"
	"go.arpabet.com/glue"
	"go.arpabet.com/raft/raftapi"
	"go.arpabet.com/raft/raftpb"
	"go.arpabet.com/raft/raftvrpc"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
)

/*
AddressReconciler keeps this node's raft membership record equal to its
configured consensusdb.advertise-address.

Raft persists each voter under the address it was added with. Joiners enroll
under their advertise address already, but the seed bootstraps under its
resolved private IP (raftmod derives the transport address from
raft.bind-address), and any recorded address goes stale when the machine moves —
on Kubernetes a rescheduled pod keeps its DNS name but changes IP, leaving a
healthy voter unreachable to peers that dial the old address.

The reconciler closes that gap without operator action: once raft is up it
compares this node's recorded address with its advertise address and, on drift,
re-registers itself — via AddVoter when this node leads, otherwise via the
control-plane Join RPC against the leader (the exact call the old runbook had
the operator make). Because a stale-addressed node may not know the leader (it
receives no AppendEntries at its new address), the follower path tries every
peer in the configuration; non-leaders reject the join and the leader applies
it. The loop retries until the record matches, then stops. A node with no
advertise-address keeps today's behavior.
*/
type AddressReconciler struct {
	Log         *zap.Logger            `inject:""`
	Properties  glue.Properties        `inject:""`
	NodeService *NodeService           `inject:""`
	RaftServer  raftapi.RaftServer     `inject:""`
	Pool        raftapi.RaftClientPool `inject:"optional"`

	Interval time.Duration `value:"consensusdb.address-reconcile-interval,default=5s"`

	selfID    raft.ServerID
	advertise raft.ServerAddress

	cancel context.CancelFunc
	done   chan struct{}
}

func (t *AddressReconciler) BeanName() string { return "address-reconciler" }

func (t *AddressReconciler) PostConstruct() error {
	advertise := t.Properties.GetString("consensusdb.advertise-address", "")
	if advertise == "" || t.Interval <= 0 {
		t.Log.Info("AddressReconcilerDisabled")
		return nil
	}
	t.selfID = raft.ServerID(t.NodeService.NodeIdHex())
	t.advertise = raft.ServerAddress(advertise)
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.done = make(chan struct{})
	go t.loop(ctx)
	return nil
}

func (t *AddressReconciler) Destroy() error {
	if t.cancel != nil {
		t.cancel()
		<-t.done
	}
	return nil
}

func (t *AddressReconciler) loop(ctx context.Context) {
	defer close(t.done)
	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rf, ok := t.RaftServer.Raft()
			if !ok {
				continue // raft not started yet
			}
			healed, err := t.reconcileOnce(ctx, rf)
			if err != nil {
				t.Log.Warn("AddressReconcile", zap.Error(err))
				continue
			}
			if healed {
				return
			}
		}
	}
}

// reconcileOnce compares this node's recorded raft address with its advertise
// address and re-registers on drift. healed=true once the record matches (the
// loop then stops); after issuing a fix it returns false so the next tick
// verifies the committed configuration.
func (t *AddressReconciler) reconcileOnce(ctx context.Context, rf *raft.Raft) (healed bool, err error) {
	future := rf.GetConfiguration()
	if err := future.Error(); err != nil {
		return false, err
	}
	cfg := future.Configuration()
	recorded, found := recordedAddress(cfg, t.selfID)
	if !found {
		// Not a member yet — a fresh joiner is added by its enrollment, under the
		// advertise address already. Nothing to fix; keep watching.
		return false, nil
	}
	if recorded == t.advertise {
		t.Log.Info("NodeAddressReconciled", zap.String("address", string(t.advertise)))
		return true, nil
	}
	if rf.State() == raft.Leader {
		// AddVoter with this node's existing id updates its address in place.
		if err := rf.AddVoter(t.selfID, t.advertise, 0, 10*time.Second).Error(); err != nil {
			return false, err
		}
		t.Log.Info("NodeAddressHealed",
			zap.String("stale", string(recorded)), zap.String("advertise", string(t.advertise)), zap.String("via", "self-leader"))
		return false, nil
	}
	return false, t.joinViaPeers(ctx, rf, cfg, recorded)
}

// joinViaPeers asks the leader to re-register this node. The known leader is
// tried first, then every other peer — a node whose recorded address is stale
// receives no AppendEntries, so its own leader knowledge may be empty or wrong;
// non-leaders reject the Join and the actual leader applies it.
func (t *AddressReconciler) joinViaPeers(ctx context.Context, rf *raft.Raft, cfg raft.Configuration, recorded raft.ServerAddress) error {
	if t.Pool == nil {
		return nil
	}
	targets := make([]raft.ServerAddress, 0, len(cfg.Servers))
	if leaderAddr, _ := rf.LeaderWithID(); leaderAddr != "" {
		targets = append(targets, leaderAddr)
	}
	for _, s := range cfg.Servers {
		if s.ID != t.selfID {
			targets = append(targets, s.Address)
		}
	}
	var lastErr error
	for _, addr := range targets {
		connAny, err := t.Pool.GetAPIConn(addr)
		if err != nil {
			lastErr = err
			continue
		}
		cli, ok := connAny.(valueclient.Client)
		if !ok {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err = raftvrpc.CallJoin(callCtx, cli, &raftpb.RaftNode{NodeId: string(t.selfID), NodeAddr: string(t.advertise)})
		cancel()
		if err == nil {
			t.Log.Info("NodeAddressHealed",
				zap.String("stale", string(recorded)), zap.String("advertise", string(t.advertise)), zap.String("via", string(addr)))
			return nil
		}
		lastErr = err
	}
	return lastErr
}

// recordedAddress returns the address the configuration holds for id.
func recordedAddress(cfg raft.Configuration, id raft.ServerID) (raft.ServerAddress, bool) {
	for _, s := range cfg.Servers {
		if s.ID == id {
			return s.Address, true
		}
	}
	return "", false
}
