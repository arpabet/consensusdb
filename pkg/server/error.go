/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import "github.com/pkg/errors"

var (
	ErrorIndexOutOfBounds = errors.New("index out of bounds")
	ErrorEmptyKey = errors.New("empty key")
	ErrorWrongSize = errors.New("wrong size")
	ErrorUnsupportedOperation = errors.New("unsupported operation")
)

