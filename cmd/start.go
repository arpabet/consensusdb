/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"

	"go.arpabet.com/cligo"
)

type StartCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *StartCommand) Command() string { return "start" }

func (t *StartCommand) Help() (string, string) { return "start server", "" }

func (t *StartCommand) Run(ctx context.Context) error {
	return nil
}
