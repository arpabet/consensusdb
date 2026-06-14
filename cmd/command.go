/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"flag"
	"fmt"
)

/**
	Alex Shvid
 */

type commandFace interface {

	Run(args []string) error

	Desc() string

}

var allCommands = map[string]commandFace {

	"version": &versionCommand{},

	"seal": &sealCommand{},

	"unseal": &unsealCommand{},

	"start": &startCommand{},

	"run": &runCommand{},

	"stop": &stopCommand{},

	"licenses": &licensesCommand{},

	"help": &helpCommand{},

}

func preprocessArgs(args []string) []string {

	if len(args) == 1 && (args[0] == "-v" || args[0] == "-version" || args[0] == "--version") {
		return []string{"version"}
	}

	return args
}

func printUsage() {

	fmt.Println("Usage: consensusdb [command]")

	for name, command := range allCommands {
		fmt.Printf("    %s - %s\n", name, command.Desc())
	}

	fmt.Println("Flags:")
	flag.PrintDefaults()

}

func Run(args []string) int {

	args = preprocessArgs(args)

	if len(args) >= 1 {

		cmd := args[0]

		if inst, ok := allCommands[cmd]; ok {

			if err := inst.Run(args[1:]); err != nil {
				fmt.Printf("Error: %v\n", err)
				return 1
			}
			return 0

		} else {
			fmt.Printf("Invalid command: %s\n", cmd)
			printUsage()
			return 1
		}

	} else {
		printUsage()
		return 0
	}
}



