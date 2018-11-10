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

package bbserver

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"crypto/rand"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"github.com/bigbagger/bigbagger/proto/bbproto"
)

type ICipher interface {

	Create(key []byte) (cipher.Block, error)

}

var KnownCiphers = map[bbproto.Cipher]ICipher {
	bbproto.Cipher_CIPHER_AES: &AesCipher{},
}


type IBlockMode interface {

	Encrypt(block cipher.Block, plaintext[]byte) ([]byte, error)

	Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error)

}

var KnownBlockModes = map[bbproto.BlockMode]IBlockMode {
	bbproto.BlockMode_MODE_GCM: &GCM{},
	bbproto.BlockMode_MODE_CFB: &CFB{},
}

type GCM struct {
}

func (this *GCM) Encrypt(block cipher.Block, plaintext[]byte) ([]byte, error) {

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

func (this *GCM) Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error) {

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]

	return gcm.Open(nil, nonce, encrypted, nil)

}

type CFB struct {
}

func (this *CFB) Encrypt(block cipher.Block, plaintext []byte) ([]byte, error) {

	blockSize := block.BlockSize()

	ciphertext := make([]byte, blockSize + len(plaintext))
	iv := ciphertext[:blockSize]

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[blockSize:], plaintext)

	return ciphertext, nil
}

func (this *CFB) Decrypt(block cipher.Block, ciphertext []byte) ([]byte, error) {

	blockSize := block.BlockSize()

	iv, encrypted := ciphertext[:blockSize], ciphertext[blockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)

	plaintext := make([]byte, len(encrypted))
	stream.XORKeyStream(plaintext, encrypted)

	return plaintext, nil
}

type AesCipher struct {
}

func (this *AesCipher) Create(key []byte) (cipher.Block, error) {
	return aes.NewCipher(key)
}

func GetPasswordHash(password string) ([]byte, error) {

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return hash, nil

}

func GetBlockKey(hash []byte, len int32) []byte {
	return hash[:len]
}