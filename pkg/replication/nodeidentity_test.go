/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
)

func TestNodeIdentityFileRoundTrip(t *testing.T) {
	rec, err := iam.GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := rec.Load()
	keyPEM, pub, _ := iam.GenerateLeafKey()
	certPEM, err := ca.Sign(&iam.CertRequest{PublicKey: pub, CommonName: "node-1", Server: true, Client: true, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	id := &NodeIdentity{CertPEM: certPEM, KeyPEM: keyPEM, CAPEM: rec.CertPEM}

	dir := t.TempDir()
	if _, ok := LoadNodeIdentity(dir); ok {
		t.Fatal("empty data dir must not yield an identity")
	}
	if err := id.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadNodeIdentity(dir)
	if !ok {
		t.Fatal("identity not loaded after save")
	}
	if string(got.CertPEM) != string(certPEM) || string(got.KeyPEM) != string(keyPEM) || string(got.CAPEM) != string(rec.CertPEM) {
		t.Fatal("round trip mismatch")
	}
	if _, err := got.ServerTLSConfig(); err != nil {
		t.Fatalf("server tls: %v", err)
	}
	if _, err := got.ClientTLSConfig(); err != nil {
		t.Fatalf("client tls: %v", err)
	}
}

// EnrollNode generates a keypair+CSR, posts it with the token, and returns the
// signed identity — verified end to end against a stand-in enroll endpoint.
func TestEnrollNode(t *testing.T) {
	rec, _ := iam.GenerateCA()
	ca, _ := rec.Load()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Token, NodeId, RaftAddr, CsrPem string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Token == "" || req.NodeId == "" || req.CsrPem == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing fields"})
			return
		}
		csr, err := iam.ParseCSR([]byte(req.CsrPem))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		certPEM, _ := ca.Sign(&iam.CertRequest{PublicKey: csr.PublicKey, CommonName: req.NodeId, Server: true, Client: true, TTL: time.Hour})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"certPem": string(certPEM), "caPem": string(rec.CertPEM)})
	}))
	defer srv.Close()

	id, err := EnrollNode(context.Background(), srv.URL, "join-abc", "node-7", "10.0.0.7:8300", "10.0.0.7")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// The returned cert chains to the CA and matches the locally-kept key.
	caB, _ := pem.Decode(id.CAPEM)
	caCert, _ := x509.ParseCertificate(caB.Bytes)
	leafB, _ := pem.Decode(id.CertPEM)
	leaf, _ := x509.ParseCertificate(leafB.Bytes)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatalf("enrolled cert does not chain to CA: %v", err)
	}
	if _, err := tls.X509KeyPair(id.CertPEM, id.KeyPEM); err != nil {
		t.Fatalf("enrolled cert/key mismatch: %v", err)
	}

	// A rejection surfaces as an error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid join token"})
	}))
	defer bad.Close()
	if _, err := EnrollNode(context.Background(), bad.URL, "nope", "node-8", "10.0.0.8:8300", ""); err == nil {
		t.Fatal("enroll must fail on a rejected token")
	}
}

// GenesisIdentity self-issues a node cert from a fresh CA: it chains to that CA,
// carries server+client EKU and the advertised IP SAN, and round-trips to files.
func TestGenesisIdentity(t *testing.T) {
	id, caRec, err := GenesisIdentity("node-1", []string{"10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(id.CAPEM) != string(caRec.CertPEM) {
		t.Fatal("identity CA does not match returned CA record")
	}
	caB, _ := pem.Decode(id.CAPEM)
	caCert, _ := x509.ParseCertificate(caB.Bytes)
	leafB, _ := pem.Decode(id.CertPEM)
	leaf, err := x509.ParseCertificate(leafB.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	for _, ku := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{ku}}); err != nil {
			t.Fatalf("genesis node cert not valid for %v: %v", ku, err)
		}
	}
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "10.0.0.1" {
		t.Fatalf("IP SANs = %v", leaf.IPAddresses)
	}
	// The cluster-wide SAN peers verify against (survives node IP changes).
	sanOK := false
	for _, d := range leaf.DNSNames {
		if d == iam.NodeSANDNS {
			sanOK = true
		}
	}
	if !sanOK {
		t.Fatalf("DNS SANs = %v, want %s present", leaf.DNSNames, iam.NodeSANDNS)
	}
	// The CA record loads as a working signer (used later on the leader to enroll peers).
	if _, err := caRec.Load(); err != nil {
		t.Fatalf("genesis CA not loadable: %v", err)
	}
	// And the material builds mutual-TLS configs after a file round trip.
	dir := t.TempDir()
	if err := id.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadNodeIdentity(dir)
	if !ok {
		t.Fatal("genesis identity not reloadable")
	}
	if _, err := got.ServerTLSConfig(); err != nil {
		t.Fatal(err)
	}
}
