/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package constants

import "flag"

var (
	yamlFile = flag.String("conf", "consensus.yaml", "Yaml file for initialization")
)

func ParseFlags() {
	flag.Parse()
}

func GetConfigFile() string {
	return *yamlFile
}

