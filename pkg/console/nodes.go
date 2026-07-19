/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"go.arpabet.com/consensusdb/pkg/replication"
)

/*
Cluster node management for the admin console: list the raft members with their
health and per-node CPU / memory / storage load, and add or remove a voter. The
node serving the console fans out to each peer's /api/node/metrics (forwarding the
caller's credential) to collect load; membership changes are applied on the raft
leader (this node when it leads, otherwise proxied to the leader). This makes a
Kubernetes cluster manageable from the UI: watch load, scale in a new pod and join
it, or drain and remove one.
*/

// NodeMetrics is one node's resource load. Percentages are 0..100.
type NodeMetrics struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemPercent     float64 `json:"memPercent"`
	MemUsedBytes   uint64  `json:"memUsedBytes"`
	MemTotalBytes  uint64  `json:"memTotalBytes"`
	DiskPercent    float64 `json:"diskPercent"`
	DiskUsedBytes  uint64  `json:"diskUsedBytes"`
	DiskTotalBytes uint64  `json:"diskTotalBytes"`
	StoreBytes     int64   `json:"storeBytes"` // badger on-disk size
}

// Node is a cluster member with its health and (when reachable) load.
type Node struct {
	ID      string       `json:"id"`
	Address string       `json:"address"` // raft address
	Voter   bool         `json:"voter"`
	Leader  bool         `json:"leader"`
	Self    bool         `json:"self"`
	Up      bool         `json:"up"`
	Metrics *NodeMetrics `json:"metrics,omitempty"`
	Detail  string       `json:"detail,omitempty"` // e.g. why it's down
}

// localMetrics samples this node's CPU / memory / storage load. CPU% is measured
// since the previous call (0 on the first), which suits a polled dashboard.
func (t *ConsoleHandler) localMetrics() NodeMetrics {
	m := NodeMetrics{}
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		m.CPUPercent = round1(pcts[0])
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		m.MemPercent = round1(vm.UsedPercent)
		m.MemUsedBytes = vm.Used
		m.MemTotalBytes = vm.Total
	}
	if du, err := disk.Usage(t.dataDir()); err == nil {
		m.DiskPercent = round1(du.UsedPercent)
		m.DiskUsedBytes = du.Used
		m.DiskTotalBytes = du.Total
	}
	if sizer, ok := t.Storage.(interface {
		DiskSize() (int64, int64)
	}); ok {
		lsm, vlog := sizer.DiskSize()
		m.StoreBytes = lsm + vlog
	}
	return m
}

func (t *ConsoleHandler) dataDir() string {
	if t.DataDir != "" {
		return t.DataDir
	}
	return "/"
}

// nodeMetricsEndpoint serves this node's own load (peers call it for aggregation).
func (t *ConsoleHandler) nodeMetricsEndpoint(w http.ResponseWriter) {
	m := t.localMetrics()
	writeJSON(w, http.StatusOK, m)
}

// clusterNodes returns every raft member with health and load. It reads the raft
// configuration, marks self/leader, and fetches each peer's metrics over HTTP
// (deriving the peer's http address from its raft address + this node's http
// port), forwarding the caller's Authorization so peers authorize identically.
func (t *ConsoleHandler) clusterNodes(w http.ResponseWriter, r *http.Request) {
	r2, ok := t.raftHandle()
	if !ok {
		// Single-node (raft off): report just this node.
		writeJSON(w, http.StatusOK, map[string]any{
			"replication": false,
			"nodes":       []Node{{ID: "local", Self: true, Leader: true, Up: true, Voter: true, Metrics: ptr(t.localMetrics())}},
		})
		return
	}

	cfg := r2.GetConfiguration()
	if err := cfg.Error(); err != nil {
		writeErr(w, http.StatusInternalServerError, "read configuration: "+err.Error())
		return
	}
	_, leaderID := r2.LeaderWithID()
	selfID := t.selfNodeID(r2)

	auth := r.Header.Get("Authorization")
	servers := cfg.Configuration().Servers
	nodes := make([]Node, len(servers))
	// Fan out to the peers in parallel: each probe has its own 2s timeout, so a
	// serial loop would stack timeouts (one page load per down peer) — the whole
	// sweep should cost one probe, not N.
	var wg sync.WaitGroup
	for i, srv := range servers {
		nodes[i] = Node{
			ID:      string(srv.ID),
			Address: string(srv.Address),
			Voter:   srv.Suffrage == raft.Voter,
			Leader:  srv.ID == leaderID,
			Self:    string(srv.ID) == selfID,
		}
		if nodes[i].Self {
			nodes[i].Up = true
			nodes[i].Metrics = ptr(t.localMetrics())
			continue
		}
		wg.Add(1)
		go func(n *Node, addr string) {
			defer wg.Done()
			if m, err := t.fetchPeerMetrics(addr, auth); err == nil {
				n.Up = true
				n.Metrics = m
			} else {
				n.Up = false
				n.Detail = err.Error()
			}
		}(&nodes[i], string(srv.Address))
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"replication": true, "nodes": nodes})
}

// fetchPeerMetrics calls a peer's /api/node/metrics, deriving its http address
// from its raft address (same host, this node's http port).
func (t *ConsoleHandler) fetchPeerMetrics(raftAddr, auth string) (*NodeMetrics, error) {
	host, _, err := net.SplitHostPort(raftAddr)
	if err != nil {
		host = raftAddr
	}
	url := "http://" + net.JoinHostPort(host, t.httpPort()) + "/api/node/metrics"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	m := &NodeMetrics{}
	if err := json.NewDecoder(resp.Body).Decode(m); err != nil {
		return nil, err
	}
	return m, nil
}

// addNode joins a new voter to the cluster. It must run on the leader; when this
// node is not the leader it proxies the request to the leader's http endpoint.
func (t *ConsoleHandler) addNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID  string `json:"nodeId"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil ||
		req.NodeID == "" || req.Address == "" {
		writeErr(w, http.StatusBadRequest, "nodeId and address are required")
		return
	}
	// Membership requires cluster identity, proven BEFORE the config change
	// commits: the target must complete a mutual-TLS handshake with a
	// certificate chaining to THIS cluster's CA and carrying the requested node
	// id. Without this, a mistyped or foreign address still becomes a phantom
	// voter that counts toward quorum (raft's own mTLS refuses it only after
	// the fact). Enrollment (/api/cluster/enroll) needs no preflight — its join
	// token authorizes the node before it is reachable. Checked after the
	// replication gate so a raft-less node still answers 409, not a dial error.
	if _, ok := t.raftHandle(); ok {
		if err := replication.PreflightClusterPeer(t.DataDir, req.NodeID, req.Address, 5*time.Second); err != nil {
			writeErr(w, http.StatusBadRequest, "target is not a reachable member of this cluster: "+err.Error())
			return
		}
	}
	t.membershipChange(w, r, func(rf *raft.Raft) error {
		return rf.AddVoter(raft.ServerID(req.NodeID), raft.ServerAddress(req.Address), 0, 10*time.Second).Error()
	}, "add")
}

// removeNode removes a server from the cluster (leader-only / proxied to leader).
func (t *ConsoleHandler) removeNode(w http.ResponseWriter, r *http.Request, id string) {
	t.membershipChange(w, r, func(rf *raft.Raft) error {
		return rf.RemoveServer(raft.ServerID(id), 0, 10*time.Second).Error()
	}, "remove")
}

// membershipChange applies a raft config change on the leader, proxying to the
// leader when this node does not lead.
func (t *ConsoleHandler) membershipChange(w http.ResponseWriter, r *http.Request, apply func(*raft.Raft) error, what string) {
	rf, ok := t.raftHandle()
	if !ok {
		writeErr(w, http.StatusConflict, "replication is not enabled on this node")
		return
	}
	if t.Raft.IsLeader() {
		if err := apply(rf); err != nil {
			writeErr(w, http.StatusInternalServerError, what+" failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": what + "ed"})
		return
	}
	// Not the leader: proxy to the leader's http endpoint.
	leaderAddr, _ := rf.LeaderWithID()
	if leaderAddr == "" {
		writeErr(w, http.StatusServiceUnavailable, "no leader elected")
		return
	}
	if err := t.proxyToLeader(w, r, string(leaderAddr)); err != nil {
		writeErr(w, http.StatusBadGateway, "forward to leader: "+err.Error())
	}
}

// proxyToLeader re-issues the current request against the leader's http endpoint,
// forwarding the body and Authorization, and copies the response back.
func (t *ConsoleHandler) proxyToLeader(w http.ResponseWriter, r *http.Request, leaderRaftAddr string) error {
	host, _, err := net.SplitHostPort(leaderRaftAddr)
	if err != nil {
		host = leaderRaftAddr
	}
	url := "http://" + net.JoinHostPort(host, t.httpPort()) + r.URL.Path
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, url, r.Body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a := r.Header.Get("Authorization"); a != "" {
		req.Header.Set("Authorization", a)
	}
	req.Header.Set("X-Forwarded-Leader", "1") // guard against proxy loops
	if r.Header.Get("X-Forwarded-Leader") == "1" {
		return errLoop
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 4096)
	for {
		n, e := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
		}
		if e != nil {
			break
		}
	}
	return nil
}

// --- helpers ---

func (t *ConsoleHandler) raftHandle() (*raft.Raft, bool) {
	if t.Raft == nil {
		return nil, false
	}
	return t.Raft.Raft()
}

func (t *ConsoleHandler) selfNodeID(rf *raft.Raft) string {
	// The transport's local address matches this server's raft address; find the
	// configuration entry whose address equals it.
	if t.Raft == nil {
		return ""
	}
	if tr, ok := t.Raft.Transport(); ok {
		local := string(tr.LocalAddr())
		cfg := rf.GetConfiguration()
		if cfg.Error() == nil {
			for _, s := range cfg.Configuration().Servers {
				if string(s.Address) == local {
					return string(s.ID)
				}
			}
		}
	}
	return ""
}

func (t *ConsoleHandler) httpPort() string {
	// http-server.bind-address is host:port; take the port (default 8441).
	if i := strings.LastIndexByte(t.HTTPBind, ':'); i >= 0 {
		return t.HTTPBind[i+1:]
	}
	return "8441"
}

func ptr(m NodeMetrics) *NodeMetrics { return &m }
func round1(f float64) float64       { return float64(int(f*10+0.5)) / 10 }

type stringError string

func (e stringError) Error() string { return string(e) }

var errLoop = stringError("leader-forward loop detected")

func errStatus(code int) error { return stringError(http.StatusText(code)) }

// ---------------------------------------------------------------------------
// Web/CLI cluster enrollment (join tokens + node certificate signing)
// ---------------------------------------------------------------------------

// mintJoinTokenHandler mints a single-use join token (admin-only). The operator
// starts a new node with CONSENSUSDB_JOIN_TOKEN=<token>; it enrolls, receives a
// CA-signed node cert, and is added as a voter. The CLI `consensusdb cluster
// join-token` mints the same record, so web and CLI operators have parity.
func (t *ConsoleHandler) mintJoinTokenHandler(w http.ResponseWriter, r *http.Request, principal string) {
	ttl := 30 * time.Minute
	if m := r.URL.Query().Get("ttlMinutes"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Minute
		}
	}
	token, expiresAt, err := t.mintJoinToken(r.Context(), ttl, principal)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mint join token: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"joinToken": token,
		"expiresAt": expiresAt,
		"note":      "start the new node with:  CONSENSUSDB_JOIN_TOKEN=" + token + " CONSENSUSDB_JOIN_PEER=<this-node-http> consensusdb run",
	})
}

// enrollNode is the token-authenticated endpoint a joining node calls: verify the
// join token, sign the node's CSR with the built-in CA, add it as a raft voter, and
// return the node cert + CA bundle. Membership changes run on the leader, so a
// follower proxies the whole request there.
func (t *ConsoleHandler) enrollNode(w http.ResponseWriter, r *http.Request) {
	rf, ok := t.raftHandle()
	if !ok {
		writeErr(w, http.StatusConflict, "replication is not enabled on this node")
		return
	}
	if !t.Raft.IsLeader() {
		leaderAddr, _ := rf.LeaderWithID()
		if leaderAddr == "" {
			writeErr(w, http.StatusServiceUnavailable, "no leader elected")
			return
		}
		if err := t.proxyToLeader(w, r, string(leaderAddr)); err != nil {
			writeErr(w, http.StatusBadGateway, "forward to leader: "+err.Error())
		}
		return
	}
	var req struct {
		Token     string `json:"token"`
		NodeId    string `json:"nodeId"`
		RaftAddr  string `json:"raftAddr"`
		Advertise string `json:"advertise"`
		CsrPem    string `json:"csrPem"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.NodeId == "" || req.RaftAddr == "" || req.CsrPem == "" {
		writeErr(w, http.StatusBadRequest, "nodeId, raftAddr and csrPem are required")
		return
	}
	certPEM, caPEM, err := t.signNodeEnrollment(r.Context(), req.Token, req.NodeId,
		[]string{hostOnly(req.RaftAddr), hostOnly(req.Advertise)}, []byte(req.CsrPem))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rf.AddVoter(raft.ServerID(req.NodeId), raft.ServerAddress(req.RaftAddr), 0, 10*time.Second).Error(); err != nil {
		writeErr(w, http.StatusInternalServerError, "add voter: "+err.Error())
		return
	}
	t.consumeJoinToken(r.Context(), req.Token) // single-use, only after the node is added
	writeJSON(w, http.StatusCreated, map[string]any{
		"nodeId":  req.NodeId,
		"certPem": string(certPEM),
		"caPem":   string(caPEM),
	})
}

// hostOnly strips the port from a host:port address, tolerating a bare host.
func hostOnly(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
