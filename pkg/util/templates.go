/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package util

import (
	"go.arpabet.com/consensusdb/pkg/res"
	"text/template"
)

func MustAssetTemplate(name string) *template.Template {
	asset := res.MustAsset(name)
	return template.Must(template.New(name).Parse(string(asset)))
}