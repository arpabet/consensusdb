/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"fmt"
	"net"
	"os"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/config"
	"golang.org/x/xerrors"
)

/*
InitCommand writes the durable settings file so the binary is seamless to run.

	consensusdb init                     # single-node, ~/.consensusdb/consensusdb.yaml
	consensusdb init --cluster           # seed node of a raft cluster (detects host IP)
	consensusdb init --cluster --seed=false --host 10.0.0.4   # a joiner

Single-node is the default: a usable data plane, admin console and durable data
directory, no raft. --cluster records the raft/serf bind addresses that
replication needs and detects this host's routable address so peers can reach it.
Nothing here is required — env vars and -D flags override every value — but the
file makes desktop / bare-metal installs durable and inspectable.
*/
type InitCommand struct {
	Parent  cligo.CliGroup `cli:"group=cli"`
	Cluster bool           `cli:"option=cluster,default=false,help=form or join a raft cluster (records raft/serf bind addresses)"`
	Seed    bool           `cli:"option=seed,default=true,help=cluster: this node bootstraps the cluster; joiners pass --seed=false"`
	Host    string         `cli:"option=host,default=,help=cluster: bind raft/serf to this host address (default 0.0.0.0, auto-advertise)"`
	DataDir string         `cli:"option=data-dir,default=,help=override the data directory"`
	HTTP    string         `cli:"option=http,default=,help=override the admin console bind address"`
	Vrpc    string         `cli:"option=vrpc,default=,help=override the value-rpc data-plane bind address"`
	Auth    bool           `cli:"option=auth,default=false,help=require a credential / mTLS on the data plane"`
	Out     string         `cli:"option=out,default=,help=write to this path instead of the default settings file"`
	Force   bool           `cli:"option=force,default=false,help=overwrite an existing settings file"`
}

func (t *InitCommand) Command() string { return "init" }

func (t *InitCommand) Help() (string, string) {
	return "create the durable settings file",
		"Writes ~/.consensusdb/consensusdb.yaml (or --out). Single-node by default; --cluster records the raft/serf bind addresses and detects this host's address so peers can reach it. Env vars and -D flags still override the file."
}

func (t *InitCommand) Run(ctx context.Context) error {
	path := t.Out
	if path == "" {
		path = config.DefaultConfigPath()
	}

	if _, err := os.Stat(path); err == nil && !t.Force {
		fmt.Printf("settings file already exists: %s\n", path)
		fmt.Printf("edit it directly, or re-run with --force to overwrite.\n")
		return nil
	}

	s := config.DefaultSettings()
	if t.DataDir != "" {
		s.DataDir = t.DataDir
	}
	if t.HTTP != "" {
		s.HTTPBind = t.HTTP
	}
	if t.Vrpc != "" {
		s.VrpcBind = t.Vrpc
	}
	s.AuthEnabled = t.Auth

	ip := config.OutboundIP()
	if t.Cluster {
		s.Mode = config.ModeCluster
		s.Bootstrap = t.Seed
		// Bind raft/serf to this host's routable IPv4 by default: peers must be
		// able to reach the node there, and a concrete address is also what raft
		// advertises. Falling back to 0.0.0.0 lets raft auto-detect an advertise
		// address, which is fragile on dual-stack hosts, so prefer the detected IP.
		host := t.Host
		if host == "" {
			host = ip
		}
		if host == "" {
			host = "0.0.0.0"
		}
		s.RaftBind = net.JoinHostPort(host, "8300")
		s.SerfBind = net.JoinHostPort(host, "8301")
		s.Advertise = ip
	}

	if err := s.Write(path); err != nil {
		return xerrors.Errorf("write settings %s: %w", path, err)
	}

	fmt.Printf("wrote %s\n\n", path)
	fmt.Printf("  mode:         %s\n", s.Mode)
	fmt.Printf("  data-dir:     %s\n", s.DataDir)
	fmt.Printf("  http console: %s\n", s.HTTPBind)
	fmt.Printf("  data plane:   %s\n", s.VrpcBind)
	fmt.Printf("  auth:         %t\n", s.AuthEnabled)
	if t.Cluster {
		fmt.Printf("  raft:         %s (bootstrap=%t)\n", s.RaftBind, s.Bootstrap)
		fmt.Printf("  serf:         %s\n", s.SerfBind)
		if ip != "" {
			fmt.Printf("\nThis host appears reachable at %s.\n", ip)
			if t.Seed {
				fmt.Printf("Start it, then add each joiner from this (leader) node:\n")
				fmt.Printf("  consensusdb raft join --id <joiner-node-id> --address <joiner-ip>:8300\n")
			} else {
				fmt.Printf("Start it, then on the seed/leader node run:\n")
				fmt.Printf("  consensusdb raft join --id <this-node-id> --address %s:8300\n", ip)
			}
		}
	}
	fmt.Printf("\nStart the node:  consensusdb run\n")
	return nil
}
