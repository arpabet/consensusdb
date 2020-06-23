/*
 *
 * Copyright 2020-present Arpabet Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
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

	return 0
}



