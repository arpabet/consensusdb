/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package cmd

import (
	"context"
	"fmt"
	"strings"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/util"
)

type UnsealCommand struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (t *UnsealCommand) Command() string { return "unseal" }

func (t *UnsealCommand) Help() (string, string) { return "unseal database", "" }

func (t *UnsealCommand) Run(ctx context.Context) error {

	value := util.PromptPassword("Enter master key: ")
	value = strings.TrimSpace(value)

	key, err := util.ParseMasterKey(value)
	if err != nil {
		return err
	}

	fmt.Printf("master key = %v\n", key)
	return nil
}
