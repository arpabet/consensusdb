/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/util"
	"os"
	"path/filepath"
	"runtime"
)

/*
Configuration is a glue bean that carries the storage/runtime settings injected
from properties. Network binding is owned by servion (http-server.bind-address)
and the value-rpc server (vrpc-server.bind-address); this bean only describes
where data lives on disk.

The derived directories are computed in PostConstruct and the directories are
created if missing, mirroring the behaviour of the previous yaml-based config.
*/
type Configuration struct {
	DataDir string `value:"consensusdb.data-dir,default=/tmp/consensusdb"`
	// FileIO is retained for config/flag compatibility. Under badger v4 the
	// table / value-log loading-mode toggles were removed, so this no longer
	// selects file-io vs memory-map; it is currently a no-op.
	FileIO  bool   `value:"consensusdb.file-io,default=true"`
	NumCPU  int    `value:"consensusdb.num-cpu,default=0"`

	// EncryptionKey enables badger encryption-at-rest when set: a base64
	// (RawURL) AES-256 master key, e.g. one produced by the `seal` command.
	// Empty means the store is unencrypted at rest.
	EncryptionKey string `value:"consensusdb.encryption-key,default="`

	KeyDir   string
	ValueDir string
	WalDir   string
	SnapDir  string
	LogDir   string
}

func (t *Configuration) PostConstruct() error {

	t.KeyDir = filepath.Join(t.DataDir, "key")
	t.ValueDir = filepath.Join(t.DataDir, "value")
	t.WalDir = filepath.Join(t.DataDir, "WAL")
	t.SnapDir = filepath.Join(t.DataDir, "snapshot")
	t.LogDir = filepath.Join(t.DataDir, "log")

	if t.NumCPU > 0 {
		runtime.GOMAXPROCS(t.NumCPU)
	}

	if err := os.MkdirAll(t.DataDir, 0755); err != nil {
		return err
	}

	return util.CreateDirsIfNotExist(t.KeyDir, t.ValueDir, t.WalDir, t.SnapDir, t.LogDir)
}
