/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package cdb

import (
	"golang.org/x/xerrors"
	"go.arpabet.com/uuid"
	"crypto/sha256"
)

var (
	ErrorInvalidKeyLength = xerrors.New("invalid key length")
)

type BlockKey []byte

type Keychain interface {

	GetBlockKey(majorKey []byte, timestamp uuid.UUID, keyLenBits int) (BlockKey, error)

}

type PasswordbasedKeychain struct {

	passwordHash [32]byte

}

func NewPasswordbasedKeychain(password string) (kc *PasswordbasedKeychain, err error) {
	kc = &PasswordbasedKeychain { passwordHash: sha256.Sum256([]byte(password)) }
	return kc, nil
}

func (this*PasswordbasedKeychain) GetBlockKey(majorKey []byte, timestamp uuid.UUID, keyLenBits int) (BlockKey, error) {

	keyLenBytes := keyLenBits / 8

	if keyLenBytes > len(this.passwordHash) {
		return nil, ErrorInvalidKeyLength
	}

	return CopyOf(this.passwordHash[:keyLenBytes]), nil

}

func (b BlockKey) clear() {
	size := len(b)
	for i := 0; i < size; i = i + 1 {
		b[i] = 0
	}
}




