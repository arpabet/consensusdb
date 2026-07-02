/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package res

import (
	"embed"
	"fmt"
)

//go:embed licenses.txt templates
var assets embed.FS

// Asset returns the bytes of the named embedded asset.
func Asset(name string) ([]byte, error) {
	return assets.ReadFile(name)
}

// MustAsset is like Asset but panics if the asset cannot be found.
func MustAsset(name string) []byte {
	data, err := assets.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("asset %s not found: %v", name, err))
	}
	return data
}
