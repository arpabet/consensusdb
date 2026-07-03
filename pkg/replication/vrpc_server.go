/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"

	"go.arpabet.com/glue"
	quic "go.arpabet.com/value-rpc/quic"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

var vrpcServerClass = reflect.TypeOf((*valueserver.Server)(nil)).Elem()

var memServerCounter int64

type implVrpcServerFactory struct {
	Log        *zap.Logger     `inject:""`
	Properties glue.Properties `inject:""`

	beanName string
}

/*
VrpcServerFactory produces a valueserver.Server bean named beanName, listening on
the address in <beanName>.bind-address. The address scheme selects the transport:

	tcp://host:port    plain TCP (default)
	tls://host:port    TLS over TCP; mutual TLS when <beanName>.client-auth=true
	quic://host:port   QUIC (TLS-over-UDP), fast on private networks / kubernetes
	unix:///path.sock  Unix-domain socket

For tls:// and quic:// the server certificate is loaded from <beanName>.tls-cert
and <beanName>.tls-key (PEM). When <beanName>.client-auth is true the server
requires and verifies a client certificate against the CA bundle in
<beanName>.tls-ca (mutual TLS); handlers can then authorize callers by identity.

Handlers register on the returned bean (AddFunction is concurrent-safe, so the
server runs immediately and the raft control service registers afterward).

An empty bind-address disables the vrpc endpoint on this node: the factory falls
back to a uniquely-named in-process mem server, so the bean graph still resolves
(the control server and pool inject cleanly) but nothing binds to a port.
*/
func VrpcServerFactory(beanName string) glue.FactoryBean {
	return &implVrpcServerFactory{beanName: beanName}
}

func (t *implVrpcServerFactory) Object() (interface{}, error) {
	address := t.Properties.GetString(fmt.Sprintf("%s.bind-address", t.beanName), "")

	if address == "" {
		name := fmt.Sprintf("%s-disabled-%d", t.beanName, atomic.AddInt64(&memServerCounter, 1))
		srv, err := valueserver.NewMemServer(name, t.Log)
		if err != nil {
			return nil, err
		}
		go srv.Run()
		t.Log.Info("VrpcServerDisabled", zap.String("bean", t.beanName), zap.String("mem", name))
		return srv, nil
	}

	scheme, rest := splitAddressScheme(address)

	var (
		srv       valueserver.Server
		err       error
		transport = scheme
	)
	switch scheme {
	case "tls", "quic":
		var cfg *tls.Config
		if cfg, err = t.serverTLS(); err != nil {
			return nil, err
		}
		if scheme == "tls" {
			srv, err = valueserver.NewTLSServer(rest, cfg, t.Log)
		} else {
			srv, err = quic.NewServer(rest, cfg, t.Log)
		}
	case "", "tcp", "unix":
		transport = "tcp"
		if scheme == "unix" {
			transport = "unix"
		}
		srv, err = valueserver.NewServer(address, t.Log)
	default:
		return nil, xerrors.Errorf("vrpc server %q: unsupported address scheme %q (want tcp/tls/quic/unix)", t.beanName, scheme)
	}
	if err != nil {
		return nil, err
	}
	go srv.Run()
	t.Log.Info("VrpcServerFactory",
		zap.String("bean", t.beanName),
		zap.String("address", address),
		zap.String("transport", transport))
	return srv, nil
}

// serverTLS builds the server TLS config from <bean>.tls-cert / .tls-key, adding
// client-certificate verification (mutual TLS) when <bean>.client-auth is set.
func (t *implVrpcServerFactory) serverTLS() (*tls.Config, error) {
	certFile := t.Properties.GetString(fmt.Sprintf("%s.tls-cert", t.beanName), "")
	keyFile := t.Properties.GetString(fmt.Sprintf("%s.tls-key", t.beanName), "")
	if certFile == "" || keyFile == "" {
		return nil, xerrors.Errorf("vrpc server %q: tls/quic transport requires %s.tls-cert and %s.tls-key", t.beanName, t.beanName, t.beanName)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, xerrors.Errorf("vrpc server %q: load key pair: %w", t.beanName, err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if t.Properties.GetBool(fmt.Sprintf("%s.client-auth", t.beanName), false) {
		caFile := t.Properties.GetString(fmt.Sprintf("%s.tls-ca", t.beanName), "")
		if caFile == "" {
			return nil, xerrors.Errorf("vrpc server %q: client-auth requires %s.tls-ca (CA bundle to verify client certs)", t.beanName, t.beanName)
		}
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, xerrors.Errorf("vrpc server %q: read tls-ca: %w", t.beanName, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, xerrors.Errorf("vrpc server %q: tls-ca %q has no certificates", t.beanName, caFile)
		}
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = pool
	}
	return cfg, nil
}

// splitAddressScheme splits "scheme://rest"; a bare "host:port" defaults to tcp.
func splitAddressScheme(address string) (scheme, rest string) {
	if i := strings.Index(address, "://"); i >= 0 {
		return address[:i], address[i+3:]
	}
	return "tcp", address
}

func (t *implVrpcServerFactory) ObjectType() reflect.Type { return vrpcServerClass }
func (t *implVrpcServerFactory) ObjectName() string       { return t.beanName }
func (t *implVrpcServerFactory) Singleton() bool          { return true }
