/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"time"

	"go.arpabet.com/consensusdb/pkg/pb"
	"golang.org/x/xerrors"
)

/*
consensusdb runs its own single x509 certificate authority. One CA signs every
certificate in the deployment:

  - node ⇄ node mutual TLS (raft / serf / control plane), and
  - client certificates for users and service accounts (data-plane mTLS).

The CA (self-signed root cert + signing key) lives in the sealed system tenant so
every node trusts the same root and, on the leader, can mint new certificates:

	major = "__system", region = "PKI"
	  ca            → CARecord    (the root cert + signing key, PEM)
	  join/<hash>   → JoinRecord  (a pending node join token; see the node phase)

A node's own leaf key never leaves the node (kept in its data dir); only the
signed cert and the CA cert are exchanged. Identities are carried in a cert's SAN
URI, "cdb://<kind>/<name>" (e.g. cdb://user/alice), which the cert index maps back
to a principal.
*/

const (
	// PKIRegion is the system region holding the certificate authority.
	PKIRegion = "PKI"
	// CAMinor is the record holding the built-in CA (cert + signing key).
	CAMinor = "ca"
	// JoinPrefix keys a pending node join token: join/<sha256hex(token)> → JoinRecord.
	JoinPrefix = "join/"
	// TokenPrefixJoin marks an opaque cluster join token ("join-<hex>").
	TokenPrefixJoin = "join-"

	// NodeSANDNS is the cluster-wide DNS SAN every node certificate carries, and
	// the ServerName peers verify against when they dial each other. Verifying a
	// stable name instead of the peer's IP keeps the mTLS transport working when a
	// node's address changes (a Kubernetes reschedule); membership in the cluster —
	// chaining to the one CA — is the identity that matters between nodes.
	NodeSANDNS = "node.cdb.internal"

	caCommonName = "consensusdb-ca"
	caValidity   = 10 * 365 * 24 * time.Hour
)

// JoinRecord authorizes one node to enroll: it is looked up by the presented
// join token's hash, checked for expiry, and (single-use) deleted on success.
type JoinRecord struct {
	ExpiresAt int64  `value:"expiresAt"` // unix seconds; 0 = never
	CreatedBy string `value:"createdBy"` // principal that minted the token
}

// JoinIndexKey is the PKI-region minor of the reverse index for a join token hash.
func JoinIndexKey(hash string) string { return JoinPrefix + hash }

// PKIKey addresses a record in the system tenant's PKI region.
func PKIKey(minor string) *pb.Key {
	return &pb.Key{
		MajorKey:   []byte(SystemTenant),
		RegionName: []byte(PKIRegion),
		MinorKey:   []byte(minor),
	}
}

// CARecord is the built-in certificate authority: its self-signed root cert and
// signing key, both PEM. Stored (and replicated) in the sealed system tenant, so
// every node trusts the same root and the leader can sign new certificates.
type CARecord struct {
	CertPEM   []byte `value:"certPem"`
	KeyPEM    []byte `value:"keyPem"`
	CreatedAt int64  `value:"createdAt"`
}

// CA is a loaded certificate authority ready to verify chains and sign leaves.
type CA struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertPEM []byte
}

// GenerateCA creates a fresh self-signed CA with an ECDSA P-256 signing key.
func GenerateCA() (*CARecord, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("iam: CA key: %w", err)
	}
	serial, err := randSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: caCommonName},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true, // signs only leaves, never intermediates
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, xerrors.Errorf("iam: CA cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, xerrors.Errorf("iam: CA key marshal: %w", err)
	}
	return &CARecord{
		CertPEM:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:    pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		CreatedAt: now.Unix(),
	}, nil
}

// Load parses a CARecord into a signer.
func (r *CARecord) Load() (*CA, error) {
	certBlock, _ := pem.Decode(r.CertPEM)
	if certBlock == nil {
		return nil, xerrors.New("iam: CA cert PEM invalid")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("iam: CA cert parse: %w", err)
	}
	keyBlock, _ := pem.Decode(r.KeyPEM)
	if keyBlock == nil {
		return nil, xerrors.New("iam: CA key PEM invalid")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("iam: CA key parse: %w", err)
	}
	return &CA{Cert: cert, Key: key, CertPEM: r.CertPEM}, nil
}

// CertRequest describes a leaf certificate to issue. PublicKey is the subject's
// public key, taken from a parsed CSR (the private key never reaches the signer).
type CertRequest struct {
	PublicKey  interface{}
	CommonName string
	URIs       []string // SAN URIs, e.g. "cdb://user/alice"
	DNSNames   []string
	IPs        []net.IP
	Client     bool // ExtKeyUsageClientAuth (users, service accounts, and nodes)
	Server     bool // ExtKeyUsageServerAuth (nodes and the data-plane server)
	TTL        time.Duration
}

// Sign issues a leaf certificate for req, signed by the CA, returning it PEM.
func (ca *CA) Sign(req *CertRequest) ([]byte, error) {
	serial, err := randSerial()
	if err != nil {
		return nil, err
	}
	uris := make([]*url.URL, 0, len(req.URIs))
	for _, raw := range req.URIs {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, xerrors.Errorf("iam: SAN URI %q: %w", raw, err)
		}
		uris = append(uris, u)
	}
	var eku []x509.ExtKeyUsage
	if req.Client {
		eku = append(eku, x509.ExtKeyUsageClientAuth)
	}
	if req.Server {
		eku = append(eku, x509.ExtKeyUsageServerAuth)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: req.CommonName},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(req.TTL),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  eku,
		DNSNames:     req.DNSNames,
		IPAddresses:  req.IPs,
		URIs:         uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, req.PublicKey, ca.Key)
	if err != nil {
		return nil, xerrors.Errorf("iam: sign leaf: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// ParseCSR decodes a PEM-encoded certificate request and verifies its self
// signature (proof the requester holds the private key).
func ParseCSR(csrPEM []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, xerrors.New("iam: CSR PEM invalid")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("iam: CSR parse: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, xerrors.Errorf("iam: CSR signature: %w", err)
	}
	return csr, nil
}

// GenerateLeafKey mints an ECDSA P-256 keypair for a console-issued certificate,
// returning the private key PEM (handed to the owner, never stored) and the
// public key to sign.
func GenerateLeafKey() (keyPEM []byte, pub interface{}, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("iam: leaf key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, xerrors.Errorf("iam: leaf key marshal: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), &key.PublicKey, nil
}

// GenerateCSR mints an ECDSA P-256 keypair and a PEM certificate-signing request
// for it (CN=commonName). The private key never leaves the requester; only the CSR
// is sent to be signed. Used by a joining node and by register-only client flows.
func GenerateCSR(commonName string) (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("iam: CSR key: %w", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: commonName}}, key)
	if err != nil {
		return nil, nil, xerrors.Errorf("iam: create CSR: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, xerrors.Errorf("iam: CSR key marshal: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

// CertURIForPrincipal is the canonical SAN URI a client certificate carries for a
// principal: "user:alice" → "cdb://user/alice", "serviceAccount:x" → "cdb://serviceAccount/x".
func CertURIForPrincipal(principal string) string {
	kind, name, ok := ParsePrincipal(principal)
	if !ok {
		return ""
	}
	return "cdb://" + kind + "/" + name
}

// ServerTLSConfig builds a mutual-TLS server config: present certPEM/keyPEM and
// require a client certificate that chains to caPEM.
func ServerTLSConfig(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, xerrors.Errorf("iam: server keypair: %w", err)
	}
	pool, err := certPool(caPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientTLSConfig builds a mutual-TLS client config: present certPEM/keyPEM and
// verify the server certificate against caPEM.
func ClientTLSConfig(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, xerrors.Errorf("iam: client keypair: %w", err)
	}
	pool, err := certPool(caPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// MutualTLSConfig builds one tls.Config usable for BOTH ends of a node↔node
// connection: it presents certPEM/keyPEM, verifies a peer server against caPEM
// (RootCAs, client role) and requires+verifies a peer client cert against caPEM
// (ClientCAs + RequireAndVerifyClientCert, server role). Node certs carry both
// server and client EKU, so the same material authenticates in either direction.
// serverName, when non-empty, is what dialed peers' certificates are verified
// against instead of the dial address (see NodeSANDNS).
func MutualTLSConfig(caPEM, certPEM, keyPEM []byte, serverName string) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, xerrors.Errorf("iam: node keypair: %w", err)
	}
	pool, err := certPool(caPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func certPool(caPEM []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, xerrors.New("iam: CA bundle has no certificates")
	}
	return pool, nil
}

func randSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, xerrors.Errorf("iam: serial: %w", err)
	}
	return serial, nil
}
