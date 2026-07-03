/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/constants"
)

type LicensesCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *LicensesCommand) Command() string { return "licenses" }

func (t *LicensesCommand) Help() (string, string) { return "show all licenses", "" }

func (t *LicensesCommand) Run(ctx context.Context) error {
	print(constants.GetLicenses())
	return nil
}
