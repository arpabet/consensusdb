/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/raft/raftapi"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
RaftHost drives the raftmod RaftServer lifecycle inside the cligo+servion world.
raftmod's RaftServer is a sprint.Server (Bind/Serve/Shutdown) which, in
sprintframework, is driven by the server role manager. Here we call those phases
ourselves on PostConstruct.

Replication is opt-in: it is disabled unless both raft.bind-address and
serf.bind-address are configured (raftmod's RaftServer.Bind no-ops otherwise).

Membership: a seed node (raft.bootstrap=true, the default) forms a single-voter
cluster and becomes leader; further nodes set raft.bootstrap=false and are added
to the cluster by the leader through the control-plane Join RPC (raftvrpc /
raftgrpc), which calls raft.AddVoter and lets raft replicate the config to the
new node. Serf-based auto-membership is not wired (commented out upstream in
raftmod).
*/
type RaftHost struct {
	RaftServer  raftapi.RaftServer `inject:""`
	NodeService *NodeService       `inject:""`
	Log         *zap.Logger        `inject:""`

	RaftAddress string `value:"raft.bind-address,default="`
	SerfAddress string `value:"serf.bind-address,default="`

	// Bootstrap controls first-start cluster formation. A seed node bootstraps a
	// single-voter cluster and becomes leader; additional (joiner) nodes set
	// raft.bootstrap=false and instead wait to be added by the leader via the
	// control-plane Join RPC (leader-side raft.AddVoter). On a node that already
	// has persisted raft state this is a no-op regardless.
	Bootstrap bool `value:"raft.bootstrap,default=true"`

	started bool
}

func (t *RaftHost) BeanName() string { return "raft-host" }

func (t *RaftHost) PostConstruct() error {
	if t.RaftAddress == "" || t.SerfAddress == "" {
		t.Log.Info("RaftDisabled",
			zap.String("reason", "raft.bind-address and serf.bind-address are required to enable replication"))
		return nil
	}

	if err := t.RaftServer.Bind(); err != nil {
		return errors.Wrap(err, "raft server bind")
	}
	if _, ok := t.RaftServer.Transport(); !ok {
		t.Log.Warn("RaftNotBound", zap.String("raft", t.RaftAddress), zap.String("serf", t.SerfAddress))
		return nil
	}
	if err := t.RaftServer.Serve(); err != nil {
		return errors.Wrap(err, "raft server serve")
	}
	t.started = true

	if !t.Bootstrap {
		t.Log.Info("RaftJoinMode",
			zap.String("reason", "raft.bootstrap=false; awaiting Join from the cluster leader"),
			zap.String("id", t.NodeService.NodeIdHex()))
		return nil
	}

	return t.bootstrap()
}

// bootstrap forms a single-node cluster on first start. On a node that already
// has persisted raft state this is a no-op (raft returns ErrCantBootstrap).
func (t *RaftHost) bootstrap() error {
	r, ok := t.RaftServer.Raft()
	if !ok {
		return xerrors.New("raft not initialized after Serve")
	}
	transport, _ := t.RaftServer.Transport()

	cfg := raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(t.NodeService.NodeIdHex()),
				Address:  transport.LocalAddr(),
			},
		},
	}

	future := r.BootstrapCluster(cfg)
	if err := future.Error(); err != nil {
		if errors.Is(err, raft.ErrCantBootstrap) {
			t.Log.Info("RaftAlreadyBootstrapped")
			return nil
		}
		return errors.Wrap(err, "bootstrap raft cluster")
	}
	t.Log.Info("RaftBootstrapped",
		zap.String("id", t.NodeService.NodeIdHex()),
		zap.String("address", string(transport.LocalAddr())))
	return nil
}

func (t *RaftHost) Destroy() error {
	if t.started {
		return t.RaftServer.Shutdown()
	}
	return nil
}
