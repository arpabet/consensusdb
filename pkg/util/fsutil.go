/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package util

import "os"

// CopyOf returns a copy of src (so the result is safe to retain after src is reused).
func CopyOf(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// CreateDirsIfNotExist creates each directory that does not already exist.
func CreateDirsIfNotExist(dirs ...string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err = os.Mkdir(dir, 0755); err != nil {
				return err
			}
		}
	}
	return nil
}
