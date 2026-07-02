/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package cmd

import (
	"context"

	"go.arpabet.com/cligo"
)

type StopCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *StopCommand) Command() string { return "stop" }

func (t *StopCommand) Help() (string, string) { return "stop server", "" }

func (t *StopCommand) Run(ctx context.Context) error {
	println("stop")
	return nil
}
