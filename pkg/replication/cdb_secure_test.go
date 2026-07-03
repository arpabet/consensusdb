/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.uber.org/zap"
)

// secureCerts is a CA plus a server and client leaf, all signed by the CA. The
// server leaf carries 127.0.0.1 as an IP SAN so a client verifying ServerName
// "127.0.0.1" accepts it.
type secureCerts struct {
	caPEM      []byte
	serverCert tls.Certificate
	clientCert tls.Certificate
}

func newSecureCerts(t *testing.T) secureCerts {
	t.Helper()
	caKey := mustECKey(t)
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cdb-test-ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	return secureCerts{
		caPEM:      caPEM,
		serverCert: issueLeaf(t, "127.0.0.1", true, ca, caKey),
		clientCert: issueLeaf(t, "cdb-client", false, ca, caKey),
	}
}

func issueLeaf(t *testing.T, cn string, isServer bool, ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key := mustECKey(t)
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if isServer {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf %q: %v", cn, err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	leaf, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build leaf pair: %v", err)
	}
	return leaf
}

func mustECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return key
}

// writeServerCertFiles writes the server cert/key and CA bundle to temp files and
// returns their paths (the VrpcServerFactory loads TLS material from the filesystem).
func writeServerCertFiles(t *testing.T, c secureCerts) (certFile, keyFile, caFile string) {
	t.Helper()
	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.serverCert.Certificate[0]})
	keyDER, err := x509.MarshalECPrivateKey(c.serverCert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatalf("marshal server key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")
	caFile = filepath.Join(dir, "ca.crt")
	for path, data := range map[string][]byte{certFile: certPEM, keyFile: keyPEM, caFile: c.caPEM} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return certFile, keyFile, caFile
}

func secureServer(t *testing.T, bindScheme, certFile, keyFile, caFile string) (addr string, closeFn func()) {
	t.Helper()
	tmp := t.TempDir()
	probe := &vrpcDataProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": bindScheme + "://127.0.0.1:0",
			"vrpc-server.tls-cert":     certFile,
			"vrpc-server.tls-key":      keyFile,
			"vrpc-server.tls-ca":       caFile,
			"vrpc-server.client-auth":  "true",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.VrpcDataService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)
	glueCtx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	return probe.Server.Addr().String(), func() { _ = glueCtx.Close() }
}

func clientTLS(c secureCerts, withClientCert bool) *tls.Config {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(c.caPEM)
	cfg := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	if withClientCert {
		cfg.Certificates = []tls.Certificate{c.clientCert}
	}
	return cfg
}

// Mutual TLS: the cdb provider connects over tls:// presenting a client cert the
// cluster verifies, then round-trips a value; a client WITHOUT a cert is rejected.
func TestCdbMutualTLS(t *testing.T) {
	certs := newSecureCerts(t)
	certFile, keyFile, caFile := writeServerCertFiles(t, certs)
	addr, done := secureServer(t, "tls", certFile, keyFile, caFile)
	defer done()

	ds, err := cdb.New("test", "tls://"+addr, "", "STORE", cdb.WithTLSConfig(clientTLS(certs, true)))
	if err != nil {
		t.Fatalf("connect mTLS: %v", err)
	}
	defer ds.Destroy()

	ctx := context.Background()
	if err := ds.SetRaw(ctx, []byte("k"), []byte("secret"), store.NoTTL); err != nil {
		t.Fatalf("set over mTLS: %v", err)
	}
	got, err := ds.GetRaw(ctx, []byte("k"), nil, nil, false)
	if err != nil || string(got) != "secret" {
		t.Fatalf("get over mTLS = %q err=%v, want secret", got, err)
	}

	// No client certificate → the server must reject the call (client-auth on).
	bad, err := cdb.New("nocert", "tls://"+addr, "", "STORE", cdb.WithTLSConfig(clientTLS(certs, false)))
	if err == nil {
		if _, err = bad.GetRaw(ctx, []byte("k"), nil, nil, false); err == nil {
			t.Fatal("client without certificate was allowed — mutual TLS not enforced")
		}
		_ = bad.Destroy()
	}
}

// QUIC: the same value-rpc data plane works over QUIC (TLS-over-UDP), which is the
// fast transport for private networks / kubernetes.
func TestCdbQUIC(t *testing.T) {
	certs := newSecureCerts(t)
	certFile, keyFile, caFile := writeServerCertFiles(t, certs)
	addr, done := secureServer(t, "quic", certFile, keyFile, caFile)
	defer done()

	ds, err := cdb.New("test", "quic://"+addr, "", "STORE", cdb.WithTLSConfig(clientTLS(certs, true)))
	if err != nil {
		t.Fatalf("connect quic: %v", err)
	}
	defer ds.Destroy()

	ctx := context.Background()
	if err := ds.SetRaw(ctx, []byte("k"), []byte("over-quic"), store.NoTTL); err != nil {
		t.Fatalf("set over quic: %v", err)
	}
	got, err := ds.GetRaw(ctx, []byte("k"), nil, nil, false)
	if err != nil || string(got) != "over-quic" {
		t.Fatalf("get over quic = %q err=%v, want over-quic", got, err)
	}
}
