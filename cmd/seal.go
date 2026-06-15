/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/util"
)

type SealCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *SealCommand) Command() string { return "seal" }

func (t *SealCommand) Help() (string, string) { return "generate master key for database", "" }

func (t *SealCommand) Run(ctx context.Context) error {
	println("Generated master key:")
	key, err := util.GenerateMasterKey()
	if err != nil {
		return err
	}
	println(key)
	return nil
}
