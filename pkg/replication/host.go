/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
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
ourselves on PostConstruct and bootstrap a single-node cluster so this node can
become leader and accept writes.

Replication is opt-in: it is disabled unless both raft.bind-address and
serf.bind-address are configured (raftmod's RaftServer.Bind no-ops otherwise).

Single-node note: serf-based membership is not wired (it is commented out
upstream in raftmod). Joining additional voters requires driving the serf agent
or calling raft.AddVoter explicitly; this host bootstraps one voter only.
*/
type RaftHost struct {
	RaftServer  raftapi.RaftServer `inject:""`
	NodeService *NodeService       `inject:""`
	Log         *zap.Logger        `inject:""`

	RaftAddress string `value:"raft.bind-address,default="`
	SerfAddress string `value:"serf.bind-address,default="`

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
