/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"testing"
	"time"
)

// makeCSR builds a PEM CSR for a fresh client key (as a joining node or a user's
// openssl would).
func makeCSR(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestCASignClientChainsToRoot(t *testing.T) {
	rec, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := rec.Load()
	if err != nil {
		t.Fatal(err)
	}

	csr, err := ParseCSR(makeCSR(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	uri := CertURIForPrincipal(PrincipalUser("alice"))
	if uri != "cdb://user/alice" {
		t.Fatalf("cert URI = %q", uri)
	}
	certPEM, err := ca.Sign(&CertRequest{
		PublicKey: csr.PublicKey, CommonName: "alice", URIs: []string{uri}, Client: true, TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("client cert does not chain to CA: %v", err)
	}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != uri {
		t.Fatalf("SAN URIs = %v, want [%s]", leaf.URIs, uri)
	}
	// A client cert must not satisfy server-auth verification.
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err == nil {
		t.Fatal("client cert unexpectedly valid for server auth")
	}
}

func TestParseCSRRejectsTampered(t *testing.T) {
	if _, err := ParseCSR([]byte("not a pem")); err == nil {
		t.Fatal("garbage CSR accepted")
	}
	csr := makeCSR(t, "bob")
	// Flip a byte inside the DER to break the self-signature.
	block, _ := pem.Decode(csr)
	block.Bytes[len(block.Bytes)-1] ^= 0xff
	if _, err := ParseCSR(pem.EncodeToMemory(block)); err == nil {
		t.Fatal("CSR with broken signature accepted")
	}
}

func TestNodeCertBuildsTLSConfigs(t *testing.T) {
	rec, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := rec.Load()
	if err != nil {
		t.Fatal(err)
	}
	// A node cert is both server and client (raft peers dial each other).
	keyPEM, pub, err := GenerateLeafKey()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := ca.Sign(&CertRequest{
		PublicKey: pub, CommonName: "node-1",
		DNSNames: []string{"node-1"}, IPs: []net.IP{net.ParseIP("127.0.0.1")},
		Server: true, Client: true, TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ServerTLSConfig(rec.CertPEM, certPEM, keyPEM); err != nil {
		t.Fatalf("server tls config: %v", err)
	}
	if _, err := ClientTLSConfig(rec.CertPEM, certPEM, keyPEM); err != nil {
		t.Fatalf("client tls config: %v", err)
	}
	if _, err := ServerTLSConfig([]byte("bad ca"), certPEM, keyPEM); err == nil {
		t.Fatal("server tls config accepted an empty CA pool")
	}
}
