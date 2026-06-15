/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"fmt"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/constants"
)

type VersionCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *VersionCommand) Command() string { return "version" }

func (t *VersionCommand) Help() (string, string) { return "show version", "" }

func (t *VersionCommand) Run(ctx context.Context) error {
	appInfo := constants.GetAppInfo()
	fmt.Printf("ConsensusDB [Version %s, Build %s]\n", appInfo.Version, appInfo.Build)
	fmt.Printf("%s\n", constants.Copyright)
	return nil
}
