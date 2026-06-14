/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"fmt"
	"go.arpabet.com/consensusdb/pkg/util"
	"strings"
)

type unsealCommand struct {
}

func (t *unsealCommand) Desc() string {
	return "unseal database"
}

func (t *unsealCommand) Run(args []string) error {

	value := util.PromptPassword("Enter master key: ")
	value = strings.TrimSpace(value)

	key, err := util.ParseMasterKey(value)
	if err != nil {
		return err
	}

	fmt.Printf("master key = %v\n", key)

	return nil
}
