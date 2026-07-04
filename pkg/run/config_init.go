/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"go.arpabet.com/consensusdb/pkg/config"
	"go.uber.org/zap"
)

/*
ConfigInitializer writes the durable settings file on first run so a freshly
built binary leaves behind an editable ~/.consensusdb/consensusdb.yaml. It only
writes when the file is absent, so it never clobbers an operator's file or a
cluster config produced by `consensusdb init --cluster`. Failure to write (for
example a read-only container filesystem, where env drives everything) is logged
and ignored — the node still starts.

It writes only in single-node mode. A cluster node is configured deliberately
(env or `init --cluster`), so scattering a single-node template there would be
misleading; the mode is passed in from the same decision main.go used to wire the
beans.

It is registered only in the "run" scope, so peeking commands like `version` do
not create files as a side effect.
*/
type ConfigInitializer struct {
	Log *zap.Logger `inject:""`

	path string
	mode string
}

// NewConfigInitializer builds the bean for the given settings-file path and the
// resolved run mode (config.ModeSingle / config.ModeCluster).
func NewConfigInitializer(path, mode string) *ConfigInitializer {
	return &ConfigInitializer{path: path, mode: mode}
}

func (t *ConfigInitializer) BeanName() string { return "config-initializer" }

func (t *ConfigInitializer) PostConstruct() error {
	if t.mode != config.ModeSingle {
		return nil
	}
	created, err := config.Ensure(t.path, config.DefaultSettings())
	if err != nil {
		t.Log.Warn("ConfigInitSkipped", zap.String("path", t.path), zap.Error(err))
		return nil
	}
	if created {
		t.Log.Info("ConfigCreated",
			zap.String("path", t.path),
			zap.String("hint", "single-node defaults written; edit and restart, or run `consensusdb init --cluster` to form a cluster"))
	}
	return nil
}
