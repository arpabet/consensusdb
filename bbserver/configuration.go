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
	"strconv"
	"path/filepath"
	"github.com/bigbagger/bigbagger/bbcommon"
)

type Configuration struct {

	Peers                  map[int]string
	PeerId                 int
	ClusterId              int
	PeerName			   string

	HttpAddress            string
	GrpcAddress            string

	RootDir                string
	DataDir                string
	WalDir                 string
	SnapDir                string

	SecurityContext        ISecurityContext

	CompressionEnabled     bool
	Compressor             ICompressor
	CompressionLevel       int

	EncryptionEnabled      bool
	EncryptionCipher       ICipher
	EncryptionMode         ICipherMode
	EncryptionTopo         string
	EncryptionKeyLen       int        // key length in bytes

}

func LoadConfiguration(cfg *ini.File) (*Configuration, error) {

	networkSection := cfg.Section("network")

	peers := make(map[int]string)

	for _, k := range networkSection.KeyStrings() {

		if len(k) > 5 && "peer." == k[:5] {

			id, err := strconv.Atoi(k[5:])
			if err != nil {
				return nil, err
			}

			peers[id] = networkSection.Key(k).String()

		}

	}

	peerId, err := networkSection.Key("peerId").Int()
	if err != nil {
		return nil, err
	}

	clusterId, err := networkSection.Key("clusterId").Int()
	if err != nil {
		return nil, err
	}

	var peerName string
	if networkSection.HasKey("peerName") {
		peerName = networkSection.Key("peerName").String()
	} else {
		peerName = networkSection.Key("peer." +  strconv.Itoa(peerId)).String()
	}

	serverSection := cfg.Section("server")

	httpAddress := serverSection.Key("httpAddress").String()
	grpcAddress := serverSection.Key("grpcAddress").String()

	databaseSection := cfg.Section("database")

	rootDir := databaseSection.Key("rootDir").String()

	var dataDir string
	var walDir  string
	var snapDir string

	if databaseSection.HasKey("dataDir") {
		dataDir = databaseSection.Key("dataDir").String()
	} else {
		dataDir = filepath.Join(rootDir, "data")
	}

	if databaseSection.HasKey("walDir") {
		walDir = databaseSection.Key("walDir").String()
	} else {
		walDir = filepath.Join(rootDir, "WAL")
	}

	if databaseSection.HasKey("snapDir") {
		snapDir = databaseSection.Key("snapDir").String()
	} else {
		snapDir = filepath.Join(rootDir, "snap")
	}

	if databaseSection.HasKey("createIfNotExist") {
		b, err := databaseSection.Key("createIfNotExist").Bool()
		if err != nil {
			return nil, err
		}
		if b {
			bbcommon.CreateDirsIfNotExist(rootDir, dataDir, walDir, snapDir)
		}
	}

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

		Peers: 		peers,
		PeerId: 	peerId,
		ClusterId: 	clusterId,
		PeerName: 	peerName,

		HttpAddress: httpAddress,
		GrpcAddress: grpcAddress,

		RootDir: 	rootDir,
		DataDir: 	dataDir,
		WalDir: 	walDir,
		SnapDir: 	snapDir,

		CompressionEnabled: compressionEnabled,
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

func NewDefaultConfiguration(httpAddress, grpcAddress, rootDir string) (*Configuration, error) {

	passwordMap := map[string]string {
		"password" : "De6*u1tPassw0rd!",
	}

	securityContext, err := NewSimpleSecurityContext(passwordMap)
	if err != nil {
		return nil, err
	}

	conf := &Configuration{

		Peers: map[int]string{1: httpAddress},
		PeerId: 	1,
		ClusterId: 	1,
		PeerName: 	httpAddress,

		HttpAddress: httpAddress,
		GrpcAddress: grpcAddress,

		RootDir: 	 rootDir,
		DataDir: 	 filepath.Join(rootDir, "data"),
		WalDir: 	 filepath.Join(rootDir, "WAL"),
		SnapDir: 	 filepath.Join(rootDir, "snap"),

		CompressionEnabled: true,
		Compressor: 		&LZ4Compressor{},
		CompressionLevel: 	9,

		EncryptionEnabled: true,
		EncryptionCipher: &AESCipher{},
		EncryptionMode: &CFBMode{},
		EncryptionTopo: "password",
		EncryptionKeyLen: 32,

		SecurityContext: securityContext,

	}

	bbcommon.CreateDirsIfNotExist(conf.RootDir, conf.DataDir, conf.WalDir, conf.SnapDir)

	return conf, nil
}