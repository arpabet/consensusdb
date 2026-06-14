/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd


type stopCommand struct {
}

func (t *stopCommand) Desc() string {
	return "stop server"
}

func (t *stopCommand) Run(args []string) error {

	println("stop")
	return nil
}
