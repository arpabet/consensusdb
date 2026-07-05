/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	cdb "go.arpabet.com/store/providers/cdb"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

// authProbe exposes the server (for its address) and the storage (to seed
// identity records the way `iam bootstrap` / `iam sa-add` write them).
type authProbe struct {
	Server  valueserver.Server     `inject:""`
	Storage server.KeyValueStorage `inject:""`
}

func (p *authProbe) PostConstruct() error { return nil }

func seedIdentity(t *testing.T, storage server.KeyValueStorage, minor string, obj interface{}) {
	t.Helper()
	raw, err := iam.Encode(obj)
	if err != nil {
		t.Fatalf("encode %s: %v", minor, err)
	}
	if _, err := storage.Put(&pb.RecordRequest{Key: iam.Key(minor), Value: raw}, 0); err != nil {
		t.Fatalf("seed %s: %v", minor, err)
	}
}

// requireDenied asserts the client cannot use the data plane (the handshake is
// rejected on connect, or the first call fails).
func requireDenied(t *testing.T, addr string, opts ...cdb.Option) {
	t.Helper()
	ds, err := cdb.New("denied", addr, "", "STORE", opts...)
	if err != nil {
		return // rejected at connect — good
	}
	defer ds.Destroy()
	if _, err := ds.GetRaw(context.Background(), []byte("k"), nil, nil, false); err == nil {
		t.Fatal("unauthenticated client was allowed")
	}
}

// The password/token ladder on the data plane: with auth.enabled every
// connection must present a valid credential; wrong/missing/disabled are denied.
func TestCdbAuthPasswordAndToken(t *testing.T) {
	tmp := t.TempDir()
	probe := &authProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tcp://127.0.0.1:0",
			"auth.enabled":             "true",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.AuthService{},
		&server.VrpcDataService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)
	glueCtx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer glueCtx.Close()
	addr := "tcp://" + probe.Server.Addr().String()

	// Seed identities exactly as the iam CLI would.
	adminHash, err := iam.HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	seedIdentity(t, probe.Storage, iam.UserPrefix+"admin",
		&iam.UserRecord{Name: "admin", PasswordHash: adminHash, Admin: true, CreatedAt: time.Now().Unix()})
	offHash, _ := iam.HashPassword("off")
	seedIdentity(t, probe.Storage, iam.UserPrefix+"off",
		&iam.UserRecord{Name: "off", PasswordHash: offHash, Disabled: true})
	token, tokenHash, err := iam.NewToken(iam.TokenPrefixServiceAccount)
	if err != nil {
		t.Fatal(err)
	}
	seedIdentity(t, probe.Storage, iam.ServiceAccountPrefix+"robot",
		&iam.ServiceAccountRecord{Name: "robot", TokenHash: tokenHash, CreatedAt: time.Now().Unix()})
	seedIdentity(t, probe.Storage, iam.TokenIndexKey(tokenHash),
		&iam.TokenRecord{Principal: iam.PrincipalServiceAccount("robot")})

	ctx := context.Background()

	// Password login works end to end.
	admin, err := cdb.New("admin", addr, "", "STORE",
		cdb.WithCredential(cdb.PasswordCredential("admin", "s3cret")))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Destroy()
	if err := admin.SetRaw(ctx, []byte("k"), []byte("v"), store.NoTTL); err != nil {
		t.Fatalf("authenticated set: %v", err)
	}
	if got, err := admin.GetRaw(ctx, []byte("k"), nil, nil, false); err != nil || string(got) != "v" {
		t.Fatalf("authenticated get = %q err=%v", got, err)
	}

	// Token login works end to end.
	robot, err := cdb.New("robot", addr, "", "STORE", cdb.WithCredential(cdb.TokenCredential(token)))
	if err != nil {
		t.Fatalf("robot connect: %v", err)
	}
	defer robot.Destroy()
	if got, err := robot.GetRaw(ctx, []byte("k"), nil, nil, false); err != nil || string(got) != "v" {
		t.Fatalf("token get = %q err=%v", got, err)
	}

	// Everything else is denied.
	requireDenied(t, addr) // no credential
	requireDenied(t, addr, cdb.WithCredential(cdb.PasswordCredential("admin", "wrong")))
	requireDenied(t, addr, cdb.WithCredential(cdb.PasswordCredential("ghost", "s3cret")))
	requireDenied(t, addr, cdb.WithCredential(cdb.PasswordCredential("off", "off"))) // disabled
	requireDenied(t, addr, cdb.WithCredential(cdb.TokenCredential("robot.deadbeef")))
	requireDenied(t, addr, cdb.WithCredential(cdb.TokenCredential("malformed")))
}

// issueLeafURI issues a client certificate carrying a SAN URI — the identity
// form the authenticator maps to a service account.
func issueLeafURI(t *testing.T, cn, sanURI string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key := mustECKey(t)
	uri, err := url.Parse(sanURI)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

// mTLS identity mapping: a verified client certificate whose SAN URI is
// registered authenticates without any credential; an unregistered certificate
// does not.
func TestCdbAuthMutualTLSIdentity(t *testing.T) {
	certs := newSecureCerts(t)
	certFile, keyFile, caFile := writeServerCertFiles(t, certs)

	tmp := t.TempDir()
	probe := &authProbe{}
	scan := []interface{}{
		glue.MapPropertySource{
			"consensusdb.data-dir":     tmp,
			"application.data.dir":     tmp,
			"vrpc-server.bind-address": "tls://127.0.0.1:0",
			"vrpc-server.tls-cert":     certFile,
			"vrpc-server.tls-key":      keyFile,
			"vrpc-server.tls-ca":       caFile,
			"vrpc-server.client-auth":  "true",
			"auth.enabled":             "true",
		},
		zap.NewNop(),
		&server.Configuration{},
		&server.StorageBean{},
		&server.AuthService{},
		&server.VrpcDataService{},
		probe,
	}
	scan = append(scan, replication.Beans()...)
	glueCtx, err := glue.New(scan...)
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	defer glueCtx.Close()
	addr := "tls://" + probe.Server.Addr().String()

	// Register the workload identity the way `iam sa-add --cert-idents` does.
	const ident = "urn:cdb:sa:webby"
	seedIdentity(t, probe.Storage, iam.ServiceAccountPrefix+"webby",
		&iam.ServiceAccountRecord{Name: "webby", CreatedAt: time.Now().Unix()})
	seedIdentity(t, probe.Storage, iam.CertPrefix+ident,
		&iam.CertIndexRecord{Principal: iam.PrincipalServiceAccount("webby"), CreatedAt: time.Now().Unix()})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certs.caPEM)
	uriCert := issueLeafURI(t, "webby", ident, certs.ca, certs.caKey)
	cfg := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{uriCert}, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}

	ds, err := cdb.New("webby", addr, "", "STORE", cdb.WithTLSConfig(cfg))
	if err != nil {
		t.Fatalf("mTLS identity connect: %v", err)
	}
	defer ds.Destroy()
	ctx := context.Background()
	if err := ds.SetRaw(ctx, []byte("k"), []byte("v"), store.NoTTL); err != nil {
		t.Fatalf("cert-authenticated set: %v", err)
	}

	// A CA-valid certificate with no registered identity is denied.
	requireDenied(t, addr, cdb.WithTLSConfig(clientTLS(certs, true)))
}
