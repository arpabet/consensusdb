/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package util

import (
	"crypto/rand"
	"go.arpabet.com/consensusdb/pkg/constants"
	"golang.org/x/xerrors"
	"io"
)


func GenerateMasterKey() (string, error) {
	nonce := make([]byte, constants.KeySize)
	if _, err := io.ReadFull(rand.Reader, nonce); err == nil {
		key := constants.Encoding.EncodeToString(nonce)
		return key, nil
	} else {
		return "", err
	}
}

func ParseMasterKey(base64key string) ([]byte, error) {
	key, err := constants.Encoding.DecodeString(base64key)
	if err != nil {
		return key, err
	}
	if len(key) != constants.KeySize {
		return key, xerrors.Errorf("wrong key size %d", len(key))
	}
	return key, nil
}

