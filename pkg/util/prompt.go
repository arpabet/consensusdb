/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package util

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"golang.org/x/crypto/ssh/terminal"
)

func Prompt(request string) string {
	reader := bufio.NewReader(os.Stdin)
	print(request)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func PromptPassword(request string) string {
	print(request)
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err == nil {
		println()
		password := string(bytePassword)
		return strings.TrimSpace(password)
	} else {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		return strings.TrimSpace(text)
	}
}

