/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The rendered settings file must parse back (the way glue flattens nested YAML
// into dotted keys) to the values it was written from — otherwise ResolveMode and
// glue would read something different from what `init` shows the operator.
func TestRenderParseRoundTrip(t *testing.T) {
	s := Settings{
		Mode:      ModeCluster,
		DataDir:   "/var/lib/consensusdb/data",
		HTTPBind:  "0.0.0.0:8441",
		VrpcBind:  "0.0.0.0:8444",
		RaftBind:  "10.0.0.4:8300",
		SerfBind:  "10.0.0.4:8301",
		Bootstrap: true,
		Advertise: "10.0.0.4",
	}
	flat := parseYAML(t, s.Render())

	for key, want := range map[string]string{
		"consensusdb.mode":         ModeCluster,
		"consensusdb.data-dir":     "/var/lib/consensusdb/data",
		"http-server.bind-address": "0.0.0.0:8441",
		"vrpc-server.bind-address": "0.0.0.0:8444",
		"raft.bind-address":        "10.0.0.4:8300",
		"serf.bind-address":        "10.0.0.4:8301",
		"raft.bootstrap":           "true",
	} {
		if got := flat[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// A single-node settings file must not carry any raft section, so a single node
// never accidentally wires the replication stack.
func TestSingleNodeHasNoRaft(t *testing.T) {
	out := DefaultSettings().Render()
	if strings.Contains(out, "raft:") || strings.Contains(out, "serf:") {
		t.Fatalf("single-node settings unexpectedly contains a raft/serf section:\n%s", out)
	}
	if flat := parseYAML(t, out); flat["consensusdb.mode"] != ModeSingle {
		t.Fatalf("mode = %q, want single", flat["consensusdb.mode"])
	}
}

func TestResolveMode(t *testing.T) {
	dir := t.TempDir()

	single := filepath.Join(dir, "single.yaml")
	if err := DefaultSettings().Write(single); err != nil {
		t.Fatal(err)
	}
	cluster := filepath.Join(dir, "cluster.yaml")
	cs := DefaultSettings()
	cs.Mode = ModeCluster
	cs.RaftBind = "10.0.0.4:8300"
	cs.SerfBind = "10.0.0.4:8301"
	if err := cs.Write(cluster); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist.yaml")

	tests := []struct {
		name string
		env  map[string]string
		path string
		want string
	}{
		{"no file, no env", nil, missing, ModeSingle},
		{"single file", nil, single, ModeSingle},
		{"cluster file", nil, cluster, ModeCluster},
		{"env mode overrides single file", map[string]string{"CONSENSUSDB_MODE": "cluster"}, single, ModeCluster},
		{"env RAFT_BIND_ADDRESS implies cluster", map[string]string{"RAFT_BIND_ADDRESS": "0.0.0.0:8300"}, missing, ModeCluster},
		{"malformed env ignored", map[string]string{"CONSENSUSDB_MODE": "nonsense"}, cluster, ModeCluster},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Isolate env: clear both keys, then set the case's.
			t.Setenv("CONSENSUSDB_MODE", "")
			t.Setenv("RAFT_BIND_ADDRESS", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := ResolveMode(tc.path); got != tc.want {
				t.Errorf("ResolveMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnsureWritesOnceThenLeavesAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "consensusdb.yaml")

	created, err := Ensure(path, DefaultSettings())
	if err != nil || !created {
		t.Fatalf("first Ensure: created=%v err=%v, want created=true", created, err)
	}
	// Mutate the file, then Ensure again: it must NOT overwrite an existing file.
	sentinel := "# operator edited\n"
	if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err = Ensure(path, DefaultSettings())
	if err != nil || created {
		t.Fatalf("second Ensure: created=%v err=%v, want created=false", created, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sentinel {
		t.Fatalf("Ensure overwrote an existing file")
	}
}

func TestHomeAndPathsHonorEnv(t *testing.T) {
	t.Setenv("CONSENSUSDB_HOME", "/opt/cdb")
	t.Setenv("CONSENSUSDB_CONFIG", "")
	if got := Home(); got != "/opt/cdb" {
		t.Errorf("Home = %q, want /opt/cdb", got)
	}
	if got := DefaultConfigPath(); got != "/opt/cdb/consensusdb.yaml" {
		t.Errorf("DefaultConfigPath = %q", got)
	}
	if got := DefaultDataDir(); got != "/opt/cdb/data" {
		t.Errorf("DefaultDataDir = %q", got)
	}
	t.Setenv("CONSENSUSDB_CONFIG", "/etc/cdb.yaml")
	if got := DefaultConfigPath(); got != "/etc/cdb.yaml" {
		t.Errorf("DefaultConfigPath with CONSENSUSDB_CONFIG = %q", got)
	}
}

func TestConfigFlagPaths(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"consensusdb", "run"}, nil},
		{[]string{"consensusdb", "-c", "a.yaml", "run"}, []string{"a.yaml"}},
		{[]string{"consensusdb", "--config", "b.yaml", "run"}, []string{"b.yaml"}},
		{[]string{"consensusdb", "--config=c.yaml", "run"}, []string{"c.yaml"}},
		{[]string{"consensusdb", "-c=d.yaml"}, []string{"d.yaml"}},
		{[]string{"consensusdb", "run", "-c"}, nil}, // dangling flag, no value
	}
	for _, tc := range cases {
		if got := configFlagPaths(tc.args); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("configFlagPaths(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// An explicitly passed -c cluster file must select cluster mode even when the
// default settings file is absent.
func TestResolveModeHonorsConfigFlag(t *testing.T) {
	dir := t.TempDir()
	cluster := filepath.Join(dir, "cluster.yaml")
	cs := DefaultSettings()
	cs.Mode = ModeCluster
	cs.RaftBind = "10.0.0.9:8300"
	cs.SerfBind = "10.0.0.9:8301"
	if err := cs.Write(cluster); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONSENSUSDB_MODE", "")
	t.Setenv("RAFT_BIND_ADDRESS", "")

	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"consensusdb", "-c", cluster, "run"}

	if got := ResolveMode(filepath.Join(dir, "missing.yaml")); got != ModeCluster {
		t.Errorf("ResolveMode honoring -c cluster file = %q, want cluster", got)
	}
}

// parseYAML mirrors glue's nested-map→dotted-key flattening so the test reads the
// file the same way the running node will.
func parseYAML(t *testing.T, doc string) map[string]string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(f, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return peek(f)
}
