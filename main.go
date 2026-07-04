/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package main

import (
	"os"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/cmd"
	"go.arpabet.com/consensusdb/pkg/config"
	"go.arpabet.com/consensusdb/pkg/console"
	"go.arpabet.com/consensusdb/pkg/constants"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/run"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/raft/raftvrpc"
	"go.arpabet.com/servion"
)

var (
	Version string
	Built   string
)

func main() {

	constants.SetAppInfo(Version, Built)

	// Durable settings file: written on first `run` (single-node defaults) and
	// read on every later run. Env vars and -D flags still override it
	// (priority: flags > env > config file > these in-code defaults). Kubernetes
	// can drive everything through env with no file at all.
	configPath := config.DefaultConfigPath()

	// Decide single-node vs cluster before building beans, so the raft stack is
	// wired only when clustering is actually configured (env CONSENSUSDB_MODE /
	// RAFT_BIND_ADDRESS, or the settings file). In single-node mode the raftmod
	// server is not registered at all, so servion cannot drive it to Serve an
	// unbound transport (which would panic on a fresh run).
	mode := config.ResolveMode(configPath)

	// In-code defaults (lowest priority). The seamless out-of-the-box node: an
	// admin console, a usable value-rpc data plane, and a project-local data
	// directory (./data, like gazile). Set raft.bind-address / serf.bind-address
	// (or `consensusdb init --cluster`) to switch on replication.
	properties := glue.MapPropertySource{
		// The resolved run mode, so cluster-only beans (RaftHost) can tell a real
		// cluster misconfig from a benign single-node/test build. Env
		// CONSENSUSDB_MODE still overrides.
		"consensusdb.mode":         mode,
		"http-server.bind-address": "0.0.0.0:8441",
		"http-server.options":      "handlers",
		"consensusdb.data-dir":     config.DefaultDataDir(),
		"consensusdb.file-io":      "true",
		// Encryption at rest: set to a base64 AES-256 master key (see the `seal`
		// command) to start the store encrypted. Empty means unencrypted;
		// override via config or the CONSENSUSDB_ENCRYPTION_KEY environment.
		"consensusdb.encryption-key": "",

		// value-rpc data plane for clients (and, in cluster mode, the raft control
		// plane Bootstrap/Join/GetConfiguration/ApplyCommand on the same host).
		// On by default so a fresh node is immediately usable; set empty to keep
		// it in-process. A bare host:port (no scheme) — the raft client pool
		// derives the raft↔control port offset from it via raft.rpc-bean-name.
		"vrpc-server.bind-address": "0.0.0.0:8444",
		"raft.rpc-bean-name":       "vrpc-server",

		// Where the `raft config|join|bootstrap` CLI dials the control plane.
		// Defaults to this node's own data-plane port so `consensusdb raft …`
		// works when exec'd on a running node (e.g. inside the pod).
		"raft-vrpc-client.address": "tcp://127.0.0.1:8444",

		// Data-plane authentication (etcd model): create identities with
		// `consensusdb iam bootstrap|user-add|sa-add` while disabled, then set
		// auth.enabled=true (AUTH_ENABLED=true) and restart. When enabled, every
		// connection must present a password/token credential or a registered
		// mTLS client certificate.
		"auth.enabled": "false",

		// Verifiable ledger: this node co-signs checkpoints when pointed at its
		// BLS key + CA-issued cert (see `consensusdb ledger keygen|issue`).
		// Empty disables signing; the chain digest is still served.
		"ledger.node-key":  "",
		"ledger.node-cert": "",
	}

	// "run" scope: storage + servers are only constructed when serving.
	runScope := []interface{}{
		&server.Configuration{},
		&server.StorageBean{},
		&server.AuthService{},
		&server.PolicyService{},
		servion.HttpServerScanner("http-server",
			&console.ConsoleHandler{}, // /api/* admin REST for both web apps
			// Two embedded single-page apps (pkg/webui): the read-only dashboard
			// at /dashboard, the admin console at /console, sharing /assets.
			run.NewPageHandler("/dashboard/{rest:.*}", "dashboard.html"),
			run.NewPageHandler("/console/{rest:.*}", "console.html"),
			run.NewRedirect("/", "/dashboard/"),          // homepage → dashboard
			run.NewRedirect("/dashboard", "/dashboard/"), // bare → slash
			run.NewRedirect("/console", "/console/"),     // bare → slash
			run.NewAssetsHandler(),                       // /assets/*
			servion.MetricsHandler(),
			// Plain-text "OK" health endpoints for Kubernetes probes.
			run.NewHealthHandler("/healthz"),
			run.NewHealthHandler("/livez"),
			run.NewHealthHandler("/readyz"),
		),
		// Background jobs for the console (backup ledger verification).
		console.NewJobManager(),
		// Writes the durable settings file on first run (single-node defaults);
		// no-op when it already exists. Only in the run scope, so `version`/`iam`
		// don't create files as a side effect.
		run.NewConfigInitializer(configPath, mode),
	}
	// Data-plane + sprint-bridge beans a node always needs.
	runScope = append(runScope, replication.BaseBeans()...)
	// Raft/replication beans only in cluster mode. Omitting them in single-node
	// mode is what keeps the raftmod raft server out of servion's server set, so
	// there is nothing to drive to a panicking Serve on a fresh single-node run.
	if mode == config.ModeCluster {
		runScope = append(runScope, replication.ClusterBeans()...)
	}

	// value-rpc data plane: the key-value operations over vrpc, on the same vrpc
	// host as the raft control plane (dormant when vrpc-server.bind-address empty).
	runScope = append(runScope, &server.VrpcDataService{})

	// Admin control surface: streaming backup / restore over the same vrpc host.
	runScope = append(runScope, &server.AdminService{})

	// raft + badger runtime metrics on the /metrics endpoint.
	runScope = append(runScope, &run.MetricsBridge{})

	// Root scope: lightweight beans available to every command.
	beans := []interface{}{
		properties,
		// Structured production logs (JSON to stdout) when COS=prod; human-readable
		// development logs otherwise.
		servion.ZapLogFactory(os.Getenv("COS") != "prod"),

		&cmd.VersionCommand{},
		&cmd.LicensesCommand{},
		// First-run onboarding: write the durable settings file.
		&cmd.InitCommand{},
		&cmd.SealCommand{},
		&cmd.UnsealCommand{},
		&cmd.StartCommand{},
		&cmd.StopCommand{},

		// Identity + authorization: `consensusdb iam …`.
		&cmd.IamGroup{},
		&cmd.IamBootstrapCommand{},
		&cmd.IamUserAddCommand{},
		&cmd.IamSaAddCommand{},
		&cmd.IamRoleAddCommand{},
		&cmd.IamGroupSetCommand{},
		&cmd.IamBindingAddCommand{},

		// Backup / restore to a file or S3-compatible object storage.
		&cmd.BackupCommand{},
		&cmd.RestoreCommand{},

		// Verifiable ledger: CA, node keys, offline verification.
		&cmd.LedgerGroup{},
		&cmd.LedgerCAInitCommand{},
		&cmd.LedgerKeygenCommand{},
		&cmd.LedgerIssueCommand{},
		&cmd.LedgerVerifyCommand{},
		&cmd.LedgerVerifyBackupCommand{},

		servion.RunCommand(runScope...),
	}
	// Cluster management CLI: `consensusdb raft config|join|bootstrap`, dialing
	// the control plane at raft-vrpc-client.address. The published
	// raftvrpc.RaftGroup has no parent group, which cligo rejects — cmd.RaftGroup
	// stands in for it (the commands attach to any group named "raft").
	beans = append(beans, &cmd.RaftGroup{})
	for _, b := range raftvrpc.Commands() {
		if _, isGroup := b.(cligo.CliGroup); isGroup {
			continue
		}
		beans = append(beans, b)
	}

	cligo.Main(
		cligo.Name("consensusdb"),
		cligo.Title("ConsensusDB"),
		cligo.Version(Version),
		cligo.Build(Built),
		// Load the durable settings file if present (skipped silently when
		// absent). A --config/-c flag adds higher-priority files on top.
		cligo.ConfigFile(configPath),
		cligo.Beans(beans...),
	)
}
