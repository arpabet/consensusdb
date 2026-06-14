/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package main

import (
	"go.arpabet.com/consensusdb/cmd"
	"go.arpabet.com/consensusdb/pkg/constants"
	"log"
	"math/rand"
	"os"
	"time"
)

var (
	Version   string
	Built     string
)

func main() {

	constants.ParseFlags()

	log.SetPrefix(constants.ApplicationName + ": ")
	log.SetFlags(0)

	rand.Seed(time.Now().UnixNano())

	constants.SetAppInfo(Version, Built)

	os.Exit(cmd.Run(os.Args[1:]))

}
