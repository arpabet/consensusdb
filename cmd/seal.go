/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"go.arpabet.com/consensusdb/pkg/util"
)

type sealCommand struct {
}

func (t *sealCommand) Desc() string {
	return "generate master key for database"
}

func (t *sealCommand) Run(args []string) error {
	println("Generated master key:")
	if key, err := util.GenerateMasterKey(); err == nil {
		println(key)
		return nil
	} else {
		return err
	}
}
