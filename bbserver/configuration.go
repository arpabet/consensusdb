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
	"gopkg.in/ini.v1"
)

type Configuration struct {

	HttpAddress            string
	GrpcAddress            string

	DataDir                string

	SecurityContext        ISecurityContext

	CompressionEnabled     bool
	CompressionThreshold   int
	Compressor             ICompressor
	CompressionLevel       int

	EncryptionEnabled      bool
	EncryptionCipher       ICipher
	EncryptionMode         ICipherMode
	EncryptionTopo         string
	EncryptionKeyLen       int        // key length in bytes

}

func LoadConfiguration(cfg *ini.File) (*Configuration, error) {

	serverSection := cfg.Section("server")

	httpAddress := serverSection.Key("httpAddress").String()
	grpcAddress := serverSection.Key("grpcAddress").String()

	dataDir := cfg.Section("database").Key("dataDir").String()

	securityContext, err := NewSimpleSecurityContext(cfg.Section("security").KeysHash())

	if err != nil {
		return nil, err
	}

	compressionSection := cfg.Section("compression")

	compressionEnabled, compressor, err := FindCompressor(compressionSection.Key("compressor").String())
	if err != nil {
		return nil, err
	}

	level, err := compressionSection.Key("level").Int()
	if err != nil {
		return nil, err
	}

	threshold, err := compressionSection.Key("threshold").Int()
	if err != nil {
		return nil, err
	}

	encryptionSection := cfg.Section("encryption")

	encryptionEnabled, cipher, err := FindCipher(encryptionSection.Key("cipher").String())
	if err != nil {
		return nil, err
	}

	mode, err := FindCipherMode(encryptionSection.Key("mode").String())
	if err != nil {
		return nil, err
	}

	topo := encryptionSection.Key("topo").String()

	keySize, err := encryptionSection.Key("keySize").Int()
	if err != nil {
		return nil, err
	}

	keyLen, err := GetKeyLength(keySize)
	if err != nil {
		return nil, err
	}

	return &Configuration{

		HttpAddress: httpAddress,
		GrpcAddress: grpcAddress,

		DataDir: dataDir,

		CompressionEnabled: compressionEnabled,
		CompressionThreshold: threshold,
		Compressor: compressor,
		CompressionLevel: level,

		EncryptionEnabled: encryptionEnabled,
		EncryptionCipher: cipher,
		EncryptionMode: mode,
		EncryptionTopo: topo,
		EncryptionKeyLen: keyLen,

		SecurityContext: securityContext,

	}, nil
}

func FindCompressor(name string) (enabled bool, compressor ICompressor, err error) {

	if name == "" || name=="NO" {
		return false, &NoCompressor{}, nil
	}

	compressor, ok := KnownCompressors[name]

	if !ok {
		return false, &NoCompressor{}, err
	}

	return true, compressor, nil
}

func FindCipher(name string) (enabled bool, cipher ICipher, err error) {

	if name == "" || name=="NO" {
		return false, &NoCipher{}, nil
	}

	cipher, ok := KnownCiphers[name]

	if !ok {
		return false, &NoCipher{}, err
	}

	return true, cipher, nil
}

func FindCipherMode(name string) (mode ICipherMode, err error) {

	if name == "" || name=="NO" {
		return &NoCipherMode{}, nil
	}

	mode, ok := KnownCipherModes[name]

	if !ok {
		return &NoCipherMode{}, err
	}

	return mode, nil
}

func NewDefaultConfiguration(httpAddress, grpcAddress, dataDir string) (*Configuration, error) {

	passwordMap := map[string]string {
		"password" : "De6*u1tPassw0rd!",
	}

	securityContext, err := NewSimpleSecurityContext(passwordMap)
	if err != nil {
		return nil, err
	}

	return &Configuration{

		HttpAddress: httpAddress,
		GrpcAddress: grpcAddress,

		DataDir: dataDir,

		CompressionEnabled: true,
		CompressionThreshold: 100,
		Compressor: &FlateCompressor{},
		CompressionLevel: 6,

		EncryptionEnabled: true,
		EncryptionCipher: &AESCipher{},
		EncryptionMode: &CFBMode{},
		EncryptionTopo: "password",
		EncryptionKeyLen: 32,

		SecurityContext: securityContext,

	}, nil

}