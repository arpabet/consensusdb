/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cdb

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"crypto/rand"
	"github.com/pkg/errors"
)

var (
	errorCiphertextTooShort = errors.New("ciphertext too short")
)

type Cipher interface {

	MetadataFlag() int32

	KeyLengthBits() int

	Create(key []byte) (cipher.Block, error)

}

type CipherMode interface {

	MetadataFlag() int32

	Encrypt(block cipher.Block, plaintext[]byte) ([]byte, error)

	Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error)

}


var (

	NO_ENCRYPTION = &NoBlockCipher{}

	AES = &AESBlockCipher{256}

	NO_ENCRYPTION_MODE = &NoCipherMode{}

	GCM = &GCMCipherMode{}
	CFB = &CFBCipherMode{}


	KnownCiphers = map[string]Cipher{
		"AES": AES,
	}

	KnownCipherModes = map[string]CipherMode{
		"GCM": GCM,
		"CFB": CFB,
	}


)

type NoCipher struct {
}

func (this* NoCipher) BlockSize() int {
	return 1;
}

func (this* NoCipher) Encrypt(dst, src []byte) {
	copy(dst, src)
}

func (this* NoCipher) Decrypt(dst, src []byte) {
	copy(dst, src)
}

type NoBlockCipher struct {
}

func (this* NoBlockCipher) MetadataFlag() int32 {
	return 0
}

func (this* NoBlockCipher) KeyLengthBits() int {
	return 0
}

func (this *NoBlockCipher) Create(key []byte) (cipher.Block, error) {
	return &NoCipher{}, nil
}

type NoCipherMode struct {
}

func (this *NoCipherMode) MetadataFlag() int32 {
	return 0
}

func (this *NoCipherMode) Encrypt(block cipher.Block, plaintext[]byte) ([]byte, error) {
	return plaintext, nil
}

func (this *NoCipherMode) Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

type GCMCipherMode struct {
}

func (this *GCMCipherMode) MetadataFlag() int32 {
	return bitGCM
}

func (this *GCMCipherMode) Encrypt(block cipher.Block, plaintext[]byte) ([]byte, error) {

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())

	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil

}

func (this *GCMCipherMode) Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error) {

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return ciphertext, errorCiphertextTooShort
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]

	return gcm.Open(nil, nonce, encrypted, nil)

}

type CFBCipherMode struct {
}

func (this *CFBCipherMode) MetadataFlag() int32 {
	return bitCFB
}

func (this *CFBCipherMode) Encrypt(block cipher.Block, plaintext []byte) ([]byte, error) {

	blockSize := block.BlockSize()

	ciphertext := make([]byte, blockSize + len(plaintext))
	iv := ciphertext[:blockSize]

	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[blockSize:], plaintext)

	return ciphertext, nil
}

func (this *CFBCipherMode) Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error) {

	blockSize := block.BlockSize()

	if len(ciphertext) < blockSize {
		return ciphertext, errorCiphertextTooShort
	}

	iv, encrypted := ciphertext[:blockSize], ciphertext[blockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)

	plaintext := make([]byte, len(encrypted))
	stream.XORKeyStream(plaintext, encrypted)

	return plaintext, nil
}

type AESBlockCipher struct {
	keyLengthBits int
}

func (this* AESBlockCipher) MetadataFlag() int32 {
	return bitAES
}

func (this* AESBlockCipher) KeyLengthBits() int {
	return this.keyLengthBits
}

func (this *AESBlockCipher) Create(key []byte) (cipher.Block, error) {
	return aes.NewCipher(key)
}

