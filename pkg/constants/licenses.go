/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package constants

import (
	"go.arpabet.com/consensusdb/pkg/res"
	"strings"
)

func GetLicenses() string {
	if content, err := res.Asset("licenses.txt"); err == nil {
		return filterLines(string(content), ApplicationName)
	}
	return ""
}

func filterLines(content string, words ...string) string {

	var out strings.Builder

	for _, line := range strings.Split(content, "\n") {
		include := true
		for _, word := range words {
			if strings.Contains(line, word) {
				include = false
				break
			}
		}
		if include {
			out.WriteString(line)
			out.WriteRune('\n')
		}
	}

	return out.String()
}

