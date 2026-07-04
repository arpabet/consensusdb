/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package config gives the consensusdb binary a durable, self-describing settings
file so a freshly built binary is seamless to run.

On first `run` a settings file is written in the working directory (single-node
defaults); every later run reads it. `consensusdb init` writes it explicitly and,
with --cluster, detects this host's address and records the raft/serf bind
addresses that clustering needs. Environment variables and --config / -D flags
still override the file (priority: flags > env > config file > built-in defaults),
so Kubernetes can drive everything through env without a file at all.

The file is plain YAML grouped by property prefix; glue flattens nested maps into
dotted property keys (raft: { bind-address } → raft.bind-address), so the file is
both human-editable and a direct property source.
*/
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Home is the base directory for consensusdb state (settings + default data).
// $CONSENSUSDB_HOME wins; otherwise the current working directory — so state is
// project-local by default (data in ./data), the same convention as gazile.
func Home() string {
	if h := strings.TrimSpace(os.Getenv("CONSENSUSDB_HOME")); h != "" {
		return h
	}
	return "."
}

// DefaultConfigPath resolves the settings file. $CONSENSUSDB_CONFIG wins;
// otherwise <home>/consensusdb.yaml.
func DefaultConfigPath() string {
	if c := strings.TrimSpace(os.Getenv("CONSENSUSDB_CONFIG")); c != "" {
		return c
	}
	return filepath.Join(Home(), "consensusdb.yaml")
}

// DefaultDataDir is <home>/data — ./data by default (project-local, like
// gazile), and never the old /tmp default that is cleared on reboot.
func DefaultDataDir() string {
	return filepath.Join(Home(), "data")
}

// Settings is the small set of knobs the settings file records. It is not the
// full property surface (env / -D reach every property); it is the seamless
// starting point a human edits.
type Settings struct {
	Mode        string // single | cluster
	DataDir     string
	HTTPBind    string
	VrpcBind    string
	AuthEnabled bool

	// Cluster-only. Written only when Mode == "cluster".
	RaftBind  string
	SerfBind  string
	Bootstrap bool
	Advertise string // this host's routable address, recorded as guidance
}

// DefaultSettings are the seamless single-node defaults: a usable data plane and
// admin console, no replication, durable data directory.
func DefaultSettings() Settings {
	return Settings{
		Mode:        ModeSingle,
		DataDir:     DefaultDataDir(),
		HTTPBind:    "0.0.0.0:8441",
		VrpcBind:    "0.0.0.0:8444",
		AuthEnabled: false,
	}
}

const (
	ModeSingle  = "single"
	ModeCluster = "cluster"
)

// Render produces the YAML settings file with guiding comments.
func (s Settings) Render() string {
	var b strings.Builder
	b.WriteString("# ConsensusDB settings.\n")
	b.WriteString("# Written on first run; edit and restart to change. Environment variables\n")
	b.WriteString("# (CONSENSUSDB_DATA_DIR, AUTH_ENABLED, RAFT_BIND_ADDRESS, …) and -D flags\n")
	b.WriteString("# override these. See `consensusdb init --help`.\n\n")

	b.WriteString("consensusdb:\n")
	b.WriteString(fmt.Sprintf("  mode: %s\n", s.Mode))
	b.WriteString("  # single: standalone node (no raft). cluster: joins/forms a raft cluster.\n")
	b.WriteString(fmt.Sprintf("  data-dir: %s\n", yq(s.DataDir)))
	b.WriteString("  file-io: true\n")
	b.WriteString("  encryption-key: \"\"   # base64 AES-256 master key; empty = unencrypted (see `seal`)\n\n")

	b.WriteString("http-server:\n")
	b.WriteString(fmt.Sprintf("  bind-address: %s   # admin console + REST API\n\n", yq(s.HTTPBind)))

	b.WriteString("vrpc-server:\n")
	b.WriteString(fmt.Sprintf("  bind-address: %s   # value-rpc data plane for clients\n\n", yq(s.VrpcBind)))

	b.WriteString("auth:\n")
	b.WriteString(fmt.Sprintf("  enabled: %t   # require a credential / mTLS on the data plane\n", s.AuthEnabled))

	if s.Mode == ModeCluster {
		b.WriteString("\n# Replication. This node participates in a raft cluster.\n")
		if s.Advertise != "" {
			b.WriteString(fmt.Sprintf("# Detected host address: %s (peers must be able to reach this node here).\n", s.Advertise))
		}
		b.WriteString("raft:\n")
		b.WriteString(fmt.Sprintf("  bind-address: %s\n", yq(s.RaftBind)))
		b.WriteString(fmt.Sprintf("  bootstrap: %t   # true on the seed node; false on nodes that join it\n", s.Bootstrap))
		b.WriteString("serf:\n")
		b.WriteString(fmt.Sprintf("  bind-address: %s\n", yq(s.SerfBind)))
	}

	return b.String()
}

// yq double-quotes a YAML scalar so addresses/paths with ':' or '/' stay plain
// strings regardless of content.
func yq(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// Write writes the settings file, creating parent directories. It overwrites.
func (s Settings) Write(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(s.Render()), 0o644)
}

// Ensure writes the settings file only if it does not already exist. It reports
// whether it created the file. A read-only location returns the error so callers
// can treat it as non-fatal (env still drives everything).
func Ensure(path string, s Settings) (created bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	}
	if err := s.Write(path); err != nil {
		return false, err
	}
	return true, nil
}

// ResolveMode decides whether this process runs the raft/replication stack. It
// mirrors the property priority (env over file) but reads only what the bean
// wiring decision needs, before glue/cligo build the container:
//
//	env CONSENSUSDB_MODE (single|cluster) → env RAFT_BIND_ADDRESS set → the
//	default settings file and any -c/--config file's consensusdb.mode /
//	raft.bind-address → single.
func ResolveMode(configPath string) string {
	if m := normMode(os.Getenv("CONSENSUSDB_MODE")); m != "" {
		return m
	}
	if strings.TrimSpace(os.Getenv("RAFT_BIND_ADDRESS")) != "" {
		return ModeCluster
	}
	// The default settings file plus any explicitly passed -c/--config files, so
	// `run -c cluster.yaml` wires the cluster stack too.
	for _, p := range append([]string{configPath}, configFlagPaths(os.Args)...) {
		flat := peek(p)
		if normMode(flat["consensusdb.mode"]) == ModeCluster {
			return ModeCluster
		}
		if strings.TrimSpace(flat["raft.bind-address"]) != "" {
			return ModeCluster
		}
	}
	return ModeSingle
}

// configFlagPaths extracts -c/--config file paths from the process args (mirrors
// cligo's global-flag parsing), so mode detection honors an explicitly passed
// settings file, not only the default one.
func configFlagPaths(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-c" || a == "--config":
			if i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--config="):
			out = append(out, strings.TrimPrefix(a, "--config="))
		case strings.HasPrefix(a, "-c="):
			out = append(out, strings.TrimPrefix(a, "-c="))
		}
	}
	return out
}

func normMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ModeCluster:
		return ModeCluster
	case ModeSingle:
		return ModeSingle
	default:
		return ""
	}
}

// peek reads the config file and flattens it to dotted keys the same way glue
// does, so callers can read a single property before the container exists. A
// missing or malformed file yields an empty map (never an error).
func peek(configPath string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return out
	}
	var m map[string]any
	if yaml.Unmarshal(data, &m) != nil {
		return out
	}
	flatten("", m, out)
	return out
}

func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if next, ok := v.(map[string]any); ok {
			flatten(key, next, out)
		} else {
			out[key] = fmt.Sprint(v)
		}
	}
}

// OutboundIP is a best-effort guess at this host's routable IPv4 address, used
// to guide cluster setup. It opens no connection (UDP dial just picks a route)
// and falls back to the first non-loopback interface address. May return "".
func OutboundIP() string {
	if conn, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		defer conn.Close()
		if a, ok := conn.LocalAddr().(*net.UDPAddr); ok && a.IP != nil {
			return a.IP.String()
		}
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() {
				if v4 := ipn.IP.To4(); v4 != nil {
					return v4.String()
				}
			}
		}
	}
	return ""
}
