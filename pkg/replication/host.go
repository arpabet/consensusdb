/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.arpabet.com/raft/raftapi"
	"go.uber.org/zap"
)

/*
RaftHost bootstraps the single-voter cluster on a seed node.

The raftmod RaftServer is a servion.Server, so servion's server role manager owns
its Bind/Serve/Shutdown lifecycle — exactly like the http and value-rpc servers.
RaftHost deliberately does NOT drive those phases: doing so raced servion and
double-bound the raft port (servion then logged a bind "address already in use"
error on every cluster start). Its only job is cluster formation, which can only
happen once servion's Serve has created the raft node — so it watches for the node
to come up and bootstraps asynchronously.

Replication is opt-in: RaftHost (and the rest of ClusterBeans) is registered only
in cluster mode. Even so, it re-checks raft.bind-address / serf.bind-address so a
stray registration stays inert.

Membership: a seed node (raft.bootstrap=true, the default) forms a single-voter
cluster and becomes leader; further nodes set raft.bootstrap=false and are added
by the leader through the control-plane Join RPC (raftvrpc), which calls
raft.AddVoter and lets raft replicate the config to the new node.
*/
type RaftHost struct {
	RaftServer  raftapi.RaftServer `inject:""`
	NodeService *NodeService       `inject:""`
	Log         *zap.Logger        `inject:""`

	RaftAddress string `value:"raft.bind-address,default="`
	SerfAddress string `value:"serf.bind-address,default="`

	// Mode is the resolved run mode (main.go sets consensusdb.mode from
	// config.ResolveMode). In cluster mode an empty bind address is a
	// misconfiguration worth failing loudly for; when unset (tests build the full
	// bean graph without addresses) RaftHost simply stays inert.
	Mode string `value:"consensusdb.mode,default=single"`

	// Bootstrap controls first-start cluster formation. A seed node bootstraps a
	// single-voter cluster and becomes leader; additional (joiner) nodes set
	// raft.bootstrap=false and instead wait to be added by the leader via the
	// control-plane Join RPC (leader-side raft.AddVoter). On a node that already
	// has persisted raft state this is a no-op regardless.
	Bootstrap bool `value:"raft.bootstrap,default=true"`

	// ReadyTimeout bounds how long the seed waits for servion to start the raft
	// server before giving up on bootstrap (a bind failure is logged by servion).
	ReadyTimeout time.Duration `value:"raft.bootstrap-timeout,default=30s"`

	stop chan struct{}
}

func (t *RaftHost) BeanName() string { return "raft-host" }

func (t *RaftHost) PostConstruct() error {
	if t.RaftAddress == "" || t.SerfAddress == "" {
		if t.Mode == "cluster" {
			// Cluster wired but no addresses: fail loudly here (build phase), before
			// servion would otherwise drive the raft server to Serve an unbound
			// transport and panic.
			return errors.New("cluster mode requires raft.bind-address and serf.bind-address; set RAFT_BIND_ADDRESS / SERF_BIND_ADDRESS or run `consensusdb init --cluster`")
		}
		t.Log.Info("RaftDisabled",
			zap.String("reason", "raft.bind-address and serf.bind-address are required to enable replication"))
		return nil
	}
	if !t.Bootstrap {
		t.Log.Info("RaftJoinMode",
			zap.String("reason", "raft.bootstrap=false; awaiting Join from the cluster leader"),
			zap.String("id", t.NodeService.NodeIdHex()))
		return nil
	}
	// Seed node: bootstrap once servion has started the raft server.
	t.stop = make(chan struct{})
	go t.bootstrapWhenReady()
	return nil
}

// bootstrapWhenReady waits for servion's Serve to create the raft node and its
// transport, then forms the single-voter cluster. It gives up after ReadyTimeout
// (servion logs the underlying bind error if Serve never ran).
func (t *RaftHost) bootstrapWhenReady() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(t.ReadyTimeout)

	for {
		r, rok := t.RaftServer.Raft()
		transport, tok := t.RaftServer.Transport()
		if rok && tok {
			if err := t.bootstrap(r, transport); err != nil {
				t.Log.Error("RaftBootstrap", zap.Error(err))
			}
			return
		}
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				t.Log.Warn("RaftBootstrapTimeout",
					zap.Duration("waited", t.ReadyTimeout),
					zap.String("reason", "raft server did not start; check for a bind error above"))
				return
			}
		}
	}
}

// bootstrap forms a single-node cluster. On a node that already has persisted
// raft state this is a no-op (raft returns ErrCantBootstrap).
func (t *RaftHost) bootstrap(r *raft.Raft, transport raft.Transport) error {
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

// Destroy stops the bootstrap watcher if it is still waiting. servion shuts the
// raft server itself down (it owns the lifecycle).
func (t *RaftHost) Destroy() error {
	if t.stop != nil {
		close(t.stop)
		t.stop = nil
	}
	return nil
}
