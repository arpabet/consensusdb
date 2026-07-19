/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"
)

// serveNodeTLS runs a minimal raft-port stand-in: a TLS listener with the given
// node identity that completes handshakes and closes. Returns its address.
func serveNodeTLS(t *testing.T, id *NodeIdentity) string {
	t.Helper()
	cfg, err := id.MutualConfig()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, cfg)
	t.Cleanup(func() { tlsLn.Close() })
	go func() {
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				if tc, ok := c.(*tls.Conn); ok {
					_ = tc.Handshake()
				}
				_ = c.Close()
			}(conn)
		}
	}()
	return ln.Addr().String()
}

/*
Cluster identity is checkable: members of one cluster share a CA fingerprint no
other cluster has, and the membership preflight admits only targets that prove
that identity over mutual TLS — same cluster passes, a node id mismatch is
caught, a second cluster's node is refused, and a dead address is refused. This
is what keeps two clusters on one network from being joinable by accident.
*/
func TestClusterIdentityPreflight(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()

	idA, _, err := GenesisIdentity("node-a1", []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := idA.Save(dirA); err != nil {
		t.Fatal(err)
	}
	idB, _, err := GenesisIdentity("node-b1", []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := idB.Save(dirB); err != nil {
		t.Fatal(err)
	}

	// Distinct clusters, distinct stable fingerprints.
	fpA, ok := TransportCAFingerprint(dirA)
	if !ok || !strings.HasPrefix(fpA, "sha256:") {
		t.Fatalf("fingerprint A = %q ok=%v", fpA, ok)
	}
	fpB, _ := TransportCAFingerprint(dirB)
	if fpA == fpB {
		t.Fatal("two clusters produced the same identity fingerprint")
	}
	if again, _ := TransportCAFingerprint(dirA); again != fpA {
		t.Fatal("fingerprint not stable across reads")
	}
	if _, ok := TransportCAFingerprint(t.TempDir()); ok {
		t.Fatal("a dir with no pki/ material reported an identity")
	}

	addrA := serveNodeTLS(t, idA)
	addrB := serveNodeTLS(t, idB)

	// Same cluster, right identity: admitted.
	if err := PreflightClusterPeer(dirA, "node-a1", addrA, 3*time.Second); err != nil {
		t.Fatalf("same-cluster preflight failed: %v", err)
	}
	// Same cluster but the wrong claimed node id: refused by CN pinning.
	if err := PreflightClusterPeer(dirA, "node-a2", addrA, 3*time.Second); err == nil {
		t.Fatal("preflight accepted a target under someone else's node id")
	}
	// Another cluster's node: its certificate chains to a different root.
	if err := PreflightClusterPeer(dirA, "node-b1", addrB, 3*time.Second); err == nil {
		t.Fatal("preflight admitted a node of a different cluster")
	}
	// Nothing listening: refused (this is the phantom-voter case).
	if err := PreflightClusterPeer(dirA, "node-a2", "127.0.0.1:1", 1*time.Second); err == nil {
		t.Fatal("preflight admitted a dead address")
	}
}
