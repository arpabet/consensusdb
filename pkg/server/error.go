/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import "golang.org/x/xerrors"

var (
	ErrorIndexOutOfBounds = xerrors.New("index out of bounds")
	ErrorEmptyKey = xerrors.New("empty key")
	ErrorWrongSize = xerrors.New("wrong size")
	ErrorUnsupportedOperation = xerrors.New("unsupported operation")
)

