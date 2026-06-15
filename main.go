/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package main

import (
	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/cmd"
	"go.arpabet.com/consensusdb/pkg/constants"
	"go.arpabet.com/consensusdb/pkg/run"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/servion"
	serviongrpc "go.arpabet.com/servion/grpc"
)

var (
	Version string
	Built   string
)

func main() {

	constants.SetAppInfo(Version, Built)

	// Defaults; override via a properties file or environment.
	properties := glue.MapPropertySource{
		"grpc-server.bind-address": "0.0.0.0:8442",
		"grpc-server.options":      "health;reflection",
		"http-server.bind-address": "0.0.0.0:8441",
		"http-server.options":      "handlers",
		"consensusdb.data-dir":     "/tmp/consensusdb",
		"consensusdb.file-io":      "true",
	}

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

		// "run" scope: storage + servers are only constructed when serving.
		servion.RunCommand(
			&server.Configuration{},
			&server.StorageBean{},
			serviongrpc.GrpcServerScanner("grpc-server", &server.KeyValueService{}),
			servion.HttpServerScanner("http-server",
				&run.GatewayHandler{},
				&run.SwaggerHandler{},
				&run.WelcomeHandler{},
				servion.MetricsHandler(),
				servion.HealthHandler(),
			),
		),
	}

	cligo.Main(
		cligo.Name("consensusdb"),
		cligo.Title("ConsensusDB"),
		cligo.Version(Version),
		cligo.Build(Built),
		cligo.Beans(beans...),
	)
}
