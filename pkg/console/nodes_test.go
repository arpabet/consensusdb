/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Local metrics report plausible CPU / memory / storage figures and the badger
// store size, and the peer-callable /api/node/metrics returns them.
func TestNodeMetrics(t *testing.T) {
	h, _ := newConsole(t)

	m := h.localMetrics()
	if m.MemTotalBytes == 0 || m.MemPercent < 0 || m.MemPercent > 100 {
		t.Fatalf("implausible memory: %+v", m)
	}
	if m.DiskTotalBytes == 0 || m.DiskPercent < 0 || m.DiskPercent > 100 {
		t.Fatalf("implausible disk: %+v", m)
	}

	rec := do(h, http.MethodGet, "/api/node/metrics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("node metrics = %d", rec.Code)
	}
	var got NodeMetrics
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.MemTotalBytes == 0 {
		t.Fatal("node metrics endpoint returned zero memory total")
	}
}

// With replication off, the cluster-nodes endpoint reports a single self node
// that is up and leader, with its metrics — the single-node view.
func TestClusterNodesSingleNode(t *testing.T) {
	h, _ := newConsole(t) // Raft nil ⇒ single-node

	rec := do(h, http.MethodGet, "/api/cluster/nodes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cluster nodes = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Replication bool   `json:"replication"`
		Nodes       []Node `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Replication {
		t.Fatal("expected single-node (no replication)")
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(out.Nodes))
	}
	n := out.Nodes[0]
	if !n.Self || !n.Up || !n.Leader || n.Metrics == nil {
		t.Fatalf("self node = %+v", n)
	}
}

// Add / remove without replication are refused with a clear conflict (raft off).
func TestMembershipRequiresReplication(t *testing.T) {
	h, _ := newConsole(t)

	rec := do(h, http.MethodPost, "/api/cluster/nodes", []byte(`{"nodeId":"n1","address":"10.0.0.2:8300"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("add without raft = %d, want 409", rec.Code)
	}
	rec = do(h, http.MethodDelete, "/api/cluster/nodes/n1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("remove without raft = %d, want 409", rec.Code)
	}
}

// httpPort is derived from the configured bind address.
func TestHTTPPortDerivation(t *testing.T) {
	h := &ConsoleHandler{HTTPBind: "0.0.0.0:9999"}
	if h.httpPort() != "9999" {
		t.Fatalf("httpPort = %q, want 9999", h.httpPort())
	}
	if (&ConsoleHandler{}).httpPort() != "8441" {
		t.Fatal("default httpPort should be 8441")
	}
}
