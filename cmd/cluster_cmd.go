/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/config"
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
)

/*
Cluster enrollment from the CLI — parity with the console "Add node" flow, for
operators who drive terraform/scripts rather than the web UI. `cluster join-token`
mints the same single-use join token the console does; a new node redeems it by
starting with CONSENSUSDB_JOIN_TOKEN=<token> and CONSENSUSDB_JOIN_PEER=<existing
node's http URL>, receiving a CA-signed node certificate and voter membership.
Deployments where every node shares one secret (a Kubernetes StatefulSet) skip
minting entirely: see consensusdb.bootstrap-token in the README runbook.
*/

// ClusterGroup roots `consensusdb cluster …`.
type ClusterGroup struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (ClusterGroup) Group() string { return "cluster" }

func (ClusterGroup) Help() (string, string) {
	return "cluster membership and node enrollment", ""
}

// ClusterJoinTokenCommand mints a single-use, expiring node join token.
type ClusterJoinTokenCommand struct {
	Parent cligo.CliGroup `cli:"group=cluster"`
	TTL    string         `cli:"option=ttl,default=30m,help=how long the join token is valid (e.g. 30m, 24h)"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *ClusterJoinTokenCommand) Command() string { return "join-token" }

func (t *ClusterJoinTokenCommand) Help() (string, string) {
	return "mint a single-use node join token", "The new node redeems it by starting with CONSENSUSDB_JOIN_TOKEN=<token> CONSENSUSDB_JOIN_PEER=<existing-node-http>."
}

func (t *ClusterJoinTokenCommand) Run(ctx context.Context) error {
	ttl, err := time.ParseDuration(t.TTL)
	if err != nil {
		return err
	}
	token, hash, err := iam.NewToken(iam.TokenPrefixJoin)
	if err != nil {
		return err
	}
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).Unix()
	}
	raw, err := iam.Encode(&iam.JoinRecord{ExpiresAt: expiresAt, CreatedBy: "cli"})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		if err := iam.PutPKIRecord(ctx, cli, iam.JoinIndexKey(hash), raw); err != nil {
			return err
		}
		fmt.Printf("join token (valid %s, shown once): %s\n", t.TTL, token)
		fmt.Printf("start the new node with:\n  CONSENSUSDB_JOIN_TOKEN=%s CONSENSUSDB_JOIN_PEER=<existing-node-http> consensusdb run\n", token)
		return nil
	})
}

/*
ClusterIdentityCommand prints this node's cluster identity — the transport-CA
fingerprint every member of the cluster (and only its members) chains to. Two
clusters in one network show different fingerprints; a node cloned from another
cluster's backup shows the same one. Reads the local pki/ material, so it works
offline against a stopped node.
*/
type ClusterIdentityCommand struct {
	Parent  cligo.CliGroup `cli:"group=cluster"`
	DataDir string         `value:"consensusdb.data-dir,default="`
}

func (t *ClusterIdentityCommand) Command() string { return "identity" }

func (t *ClusterIdentityCommand) Help() (string, string) {
	return "print this node's cluster identity (transport-CA fingerprint)",
		"Members of one cluster share the fingerprint; compare it across nodes and deployments to tell clusters apart."
}

func (t *ClusterIdentityCommand) Run(ctx context.Context) error {
	dir := t.DataDir
	if dir == "" {
		dir = config.DefaultDataDir()
	}
	fp, ok := replication.TransportCAFingerprint(dir)
	if !ok {
		fmt.Printf("no cluster identity: %s/pki/ca.pem not found (single-node, or the node has not joined a cluster)\n", dir)
		return nil
	}
	fmt.Printf("cluster identity: %s\n", fp)
	if id, ok := replication.LoadNodeIdentity(dir); ok {
		if block, _ := pem.Decode(id.CertPEM); block != nil {
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				fmt.Printf("node id:          %s\n", cert.Subject.CommonName)
			}
		}
	}
	return nil
}
