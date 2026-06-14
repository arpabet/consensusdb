/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd


type startCommand struct {
}

func (t *startCommand) Desc() string {
	return "start server"
}

func (t *startCommand) Run(args []string) error {

	return nil
}
