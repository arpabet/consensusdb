/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"strings"
	"testing"

	// badger registers its expvars at package init; the server binary always
	// links it via storage, the test binary needs the import explicitly.
	_ "github.com/dgraph-io/badger/v4/y"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// The bridge must install once (servion's run command can rebuild the bean graph
// on restart) and surface the badger expvars — which exist from package init —
// on the default prometheus registry that servion's /metrics handler serves.
func TestMetricsBridgeInstallsOnce(t *testing.T) {
	b := &MetricsBridge{Log: zap.NewNop()}
	if err := b.PostConstruct(); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := b.PostConstruct(); err != nil {
		t.Fatalf("re-install must be tolerated: %v", err)
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	sawBadger := false
	for _, f := range families {
		if strings.HasPrefix(f.GetName(), "badger_") {
			sawBadger = true
			break
		}
	}
	if !sawBadger {
		t.Fatal("no badger_* metrics on the default registry")
	}
}
