/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"fmt"
	"go.arpabet.com/consensusdb/pkg/constants"
)


type versionCommand struct {
}

func (t *versionCommand) Desc() string {
	return "show version"
}

func (t *versionCommand) Run(args []string) error {

	appInfo := constants.GetAppInfo()
	fmt.Printf("ConsensusDB [Version %s, Build %s]\n", appInfo.Version, appInfo.Build)
	fmt.Printf("%s\n", constants.Copyright)
	return nil
}
