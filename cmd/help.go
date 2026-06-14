/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

type helpCommand struct {

}
func (t *helpCommand) Desc() string {
	return "help command"
}

func (t *helpCommand) Run(args []string) error {
	printUsage()
	return nil
}
