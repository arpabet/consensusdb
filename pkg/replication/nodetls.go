/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"crypto/tls"
	"net"
	"reflect"
	"time"

	"go.arpabet.com/glue"
	"go.arpabet.com/raft/raftmod"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

var tlsConfigClass = reflect.TypeOf((*tls.Config)(nil))

/*
NodeTLSFactory produces the "raft-transport-tls" *tls.Config bean that secures the
raft consensus transport with mutual TLS (registered only in cluster mode, so the
node↔node channel is mandatory-mTLS there). It provisions the node's identity on
first start and loads it on restart, all from <data-dir>/pki/:

  - restart: load the existing cert/key/ca files;
  - seed (raft.bootstrap=true): generate the CA and self-issue a node cert
    (genesis), staging the CA key on disk — the console's ensureCA publishes the
    record through raft on first use, so every later cert chains to this root;
  - joiner (consensusdb.join-token set): enroll against consensusdb.join-peer to
    obtain a CA-signed node cert.

The qualifier "raft-transport-tls" (raftmod's RaftServer.TlsConfig) keeps this off
the control-plane pool and the client-facing data plane, so those are unaffected.
*/
type NodeTLSFactory struct {
	Log         *zap.Logger     `inject:""`
	Properties  glue.Properties `inject:""`
	NodeService *NodeService    `inject:""`
}

// NodeTLSConfigFactory constructs the factory bean.
func NodeTLSConfigFactory() glue.FactoryBean { return &NodeTLSFactory{} }

func (t *NodeTLSFactory) Object() (interface{}, error) {
	dataDir := t.Properties.GetString("consensusdb.data-dir", "")
	nodeID := t.NodeService.NodeIdHex()
	id, err := t.provision(dataDir, nodeID)
	if err != nil {
		return nil, xerrors.Errorf("node mTLS identity: %w", err)
	}
	cfg, err := id.MutualConfig()
	if err != nil {
		return nil, err
	}
	t.Log.Info("NodeMTLSReady", zap.String("nodeId", nodeID))
	return cfg, nil
}

func (t *NodeTLSFactory) provision(dataDir, nodeID string) (*NodeIdentity, error) {
	if id, ok := LoadNodeIdentity(dataDir); ok {
		t.Log.Info("NodeIdentityLoaded", zap.String("dir", dataDir))
		return id, nil
	}
	advertised := raftmod.ReplaceToPrivateIP(t.Properties.GetString("raft.bind-address", ""))
	advHost := hostOf(advertised)

	if token := t.Properties.GetString("consensusdb.join-token", ""); token != "" {
		peer := t.Properties.GetString("consensusdb.join-peer", "")
		if peer == "" {
			return nil, xerrors.New("join requires consensusdb.join-peer (an existing node's http URL, e.g. http://10.0.0.1:8441)")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		id, err := EnrollNode(ctx, peer, token, nodeID, advertised, advHost)
		if err != nil {
			return nil, err
		}
		if err := id.Save(dataDir); err != nil {
			return nil, err
		}
		t.Log.Info("NodeEnrolled", zap.String("peer", peer))
		return id, nil
	}

	if t.Properties.GetBool("raft.bootstrap", true) {
		id, caRec, err := GenesisIdentity(nodeID, []string{advHost})
		if err != nil {
			return nil, err
		}
		if err := id.Save(dataDir); err != nil {
			return nil, err
		}
		// Stage the CA key on disk; the console's ensureCA publishes the CA record
		// through raft on first use. Writing it into local storage here would
		// diverge replica state (the FSM must apply onto identical state).
		if err := SaveGenesisCA(dataDir, caRec); err != nil {
			return nil, err
		}
		t.Log.Info("NodeGenesis", zap.String("nodeId", nodeID), zap.String("advertise", advHost))
		return id, nil
	}
	return nil, xerrors.New("cluster node has no identity: set consensusdb.join-token + consensusdb.join-peer to enroll, or raft.bootstrap=true for the seed")
}

func (t *NodeTLSFactory) ObjectType() reflect.Type { return tlsConfigClass }
func (t *NodeTLSFactory) ObjectName() string       { return "raft-transport-tls" }
func (t *NodeTLSFactory) Singleton() bool          { return true }

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
