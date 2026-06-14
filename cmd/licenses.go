/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"go.arpabet.com/consensusdb/pkg/constants"
)


type licensesCommand struct {

}
func (t *licensesCommand) Desc() string {
	return "show all licenses"
}

func (t *licensesCommand) Run(args []string) error {
	print(constants.GetLicenses())
	return nil
}



