/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package main

import (
	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/cmd"
	"go.arpabet.com/consensusdb/pkg/constants"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/run"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/servion"
)

var (
	Version string
	Built   string
)

func main() {

	constants.SetAppInfo(Version, Built)

	// Defaults; override via a properties file or environment.
	// Replication is opt-in: set raft.bind-address and serf.bind-address (e.g.
	// "0.0.0.0:8300" / "0.0.0.0:8301") to enable the raft write path. Left
	// empty here, so writes go directly to local storage (single-node, no raft).
	properties := glue.MapPropertySource{
		"http-server.bind-address": "0.0.0.0:8441",
		"http-server.options":      "handlers",
		"consensusdb.data-dir":     "/tmp/consensusdb",
		"consensusdb.file-io":      "true",
		// Encryption at rest: set to a base64 AES-256 master key (see the `seal`
		// command) to start the store encrypted. Empty means unencrypted;
		// override via config or the CONSENSUSDB_ENCRYPTION_KEY environment.
		"consensusdb.encryption-key": "",

		// value-rpc control plane (Bootstrap/Join/GetConfiguration/ApplyCommand).
		// Empty bind-address keeps it in-process (disabled); set e.g.
		// "tcp://0.0.0.0:8444" together with raft.bind-address to enable clustering.
		// raft.rpc-bean-name points the client pool at this server so it can derive
		// the raft↔control port offset.
		"vrpc-server.bind-address": "",
		"raft.rpc-bean-name":       "vrpc-server",
	}

	// "run" scope: storage + servers are only constructed when serving.
	runScope := []interface{}{
		&server.Configuration{},
		&server.StorageBean{},
		servion.HttpServerScanner("http-server",
			&run.WelcomeHandler{},
			servion.MetricsHandler(),
			servion.HealthHandler(),
		),
	}
	// Raft replication beans (dormant unless raft/serf bind-addresses are set).
	runScope = append(runScope, replication.Beans()...)

	// value-rpc data plane: the key-value operations over vrpc, on the same vrpc
	// host as the raft control plane (dormant when vrpc-server.bind-address empty).
	runScope = append(runScope, &server.VrpcDataService{})

	// Root scope: lightweight beans available to every command.
	beans := []interface{}{
		properties,
		servion.ZapLogFactory(true),

		&cmd.VersionCommand{},
		&cmd.LicensesCommand{},
		&cmd.SealCommand{},
		&cmd.UnsealCommand{},
		&cmd.StartCommand{},
		&cmd.StopCommand{},

		servion.RunCommand(runScope...),
	}

	cligo.Main(
		cligo.Name("consensusdb"),
		cligo.Title("ConsensusDB"),
		cligo.Version(Version),
		cligo.Build(Built),
		cligo.Beans(beans...),
	)
}
