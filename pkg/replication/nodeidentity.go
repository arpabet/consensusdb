/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"golang.org/x/xerrors"
)

/*
A node's mTLS material — its CA-issued leaf (cert+key) and the CA cert for the
trust pool — is kept locally in <dataDir>/pki/ so the raft/data-plane transports
can build their tls.Config before raft (and any replicated read) is available. The
CA private key is never here: signing happens on the leader from the replicated
__system/PKI/ca record. A fresh node obtains its leaf by enrolling with a join
token (EnrollNode); the seed self-issues at genesis.
*/

const (
	pkiDirName   = "pki"
	nodeCertFile = "node-cert.pem"
	nodeKeyFile  = "node-key.pem"
	caCertFile   = "ca.pem"

	// nodeSelfCertTTL is how long a node's own certificate is valid; rotation is a
	// re-enroll (or, for the seed, a re-genesis).
	nodeSelfCertTTL = 5 * 365 * 24 * time.Hour
)

// NodeIdentity is a node's locally-stored mTLS material.
type NodeIdentity struct {
	CertPEM []byte
	KeyPEM  []byte
	CAPEM   []byte
}

func pkiDir(dataDir string) string { return filepath.Join(dataDir, pkiDirName) }

// LoadNodeIdentity reads the node's mTLS files; ok=false if any is missing/empty.
func LoadNodeIdentity(dataDir string) (*NodeIdentity, bool) {
	dir := pkiDir(dataDir)
	cert, e1 := os.ReadFile(filepath.Join(dir, nodeCertFile))
	key, e2 := os.ReadFile(filepath.Join(dir, nodeKeyFile))
	ca, e3 := os.ReadFile(filepath.Join(dir, caCertFile))
	if e1 != nil || e2 != nil || e3 != nil || len(cert) == 0 || len(key) == 0 || len(ca) == 0 {
		return nil, false
	}
	return &NodeIdentity{CertPEM: cert, KeyPEM: key, CAPEM: ca}, true
}

// Save writes the node's mTLS files, the private key mode 0600.
func (id *NodeIdentity) Save(dataDir string) error {
	dir := pkiDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, nodeKeyFile), id.KeyPEM, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, nodeCertFile), id.CertPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, caCertFile), id.CAPEM, 0o644)
}

// ServerTLSConfig builds a mutual-TLS server config (raft peer / data-plane server).
func (id *NodeIdentity) ServerTLSConfig() (*tls.Config, error) {
	return iam.ServerTLSConfig(id.CAPEM, id.CertPEM, id.KeyPEM)
}

// ClientTLSConfig builds a mutual-TLS client config (dialing raft peers).
func (id *NodeIdentity) ClientTLSConfig() (*tls.Config, error) {
	return iam.ClientTLSConfig(id.CAPEM, id.CertPEM, id.KeyPEM)
}

// MutualConfig builds one tls.Config for both roles of the consensus transport
// (raft peers dial and accept each other). This is the config injected as the
// "raft-transport-tls" bean.
func (id *NodeIdentity) MutualConfig() (*tls.Config, error) {
	return iam.MutualTLSConfig(id.CAPEM, id.CertPEM, id.KeyPEM)
}

// GenesisIdentity is the seed node's first identity: it generates a brand-new CA
// and self-issues a node leaf (server+client) from it. It returns the node
// identity (to save and use) and the CA record, which the caller persists to
// __system/PKI once the node leads so that every later cert — client and node —
// chains to this same root. hosts are the node's advertised IP/DNS for the SANs.
func GenesisIdentity(nodeID string, hosts []string) (*NodeIdentity, *iam.CARecord, error) {
	caRec, err := iam.GenerateCA()
	if err != nil {
		return nil, nil, err
	}
	ca, err := caRec.Load()
	if err != nil {
		return nil, nil, err
	}
	keyPEM, pub, err := iam.GenerateLeafKey()
	if err != nil {
		return nil, nil, err
	}
	dns, ips := splitNodeHosts(append([]string{nodeID}, hosts...))
	certPEM, err := ca.Sign(&iam.CertRequest{
		PublicKey: pub, CommonName: nodeID, DNSNames: dns, IPs: ips,
		Server: true, Client: true, TTL: nodeSelfCertTTL,
	})
	if err != nil {
		return nil, nil, err
	}
	return &NodeIdentity{CertPEM: certPEM, KeyPEM: keyPEM, CAPEM: caRec.CertPEM}, caRec, nil
}

func splitNodeHosts(hosts []string) (dns []string, ips []net.IP) {
	seen := map[string]bool{}
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h == "" || seen[h] {
			continue
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		} else {
			dns = append(dns, h)
		}
	}
	return dns, ips
}

// EnrollNode redeems a join token against an existing node's console: it generates
// a keypair + CSR locally, POSTs them with the token to /api/cluster/enroll, and
// returns the CA-signed node identity. peerURL is like "http://10.0.0.1:8441".
func EnrollNode(ctx context.Context, peerURL, token, nodeID, raftAddr, advertise string) (*NodeIdentity, error) {
	keyPEM, csrPEM, err := iam.GenerateCSR(nodeID)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{
		"token": token, "nodeId": nodeID, "raftAddr": raftAddr,
		"advertise": advertise, "csrPem": string(csrPEM),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(peerURL, "/")+"/api/cluster/enroll", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, xerrors.Errorf("enroll to %s: %w", peerURL, err)
	}
	defer resp.Body.Close()
	var out struct {
		CertPem string `json:"certPem"`
		CaPem   string `json:"caPem"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusCreated || out.CertPem == "" {
		if out.Error == "" {
			out.Error = resp.Status
		}
		return nil, xerrors.Errorf("enroll rejected: %s", out.Error)
	}
	return &NodeIdentity{CertPEM: []byte(out.CertPem), KeyPEM: keyPEM, CAPEM: []byte(out.CaPem)}, nil
}
