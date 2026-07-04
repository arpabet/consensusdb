/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
AuthService is the data-plane authenticator (plan S2): it validates the
credential a client attaches to the value-rpc handshake and derives the
connection's principal, which the server then injects into every handler's
context (valuerpc.PrincipalFromContext). Authorization on top of the principal
arrives in a later phase.

The ladder, all resolving to one principal:

	{method:"password", user, pass}  → "user:<name>"            (argon2id, humans)
	{method:"token", token}          → "serviceAccount:<name>"  (API token)
	mTLS client certificate          → "serviceAccount:<name>"  (SAN URI or CN
	                                    registered in the cert index)

An explicit credential takes precedence over the peer certificate (etcd
semantics). Identities are read from the system tenant (pkg/iam layout) on the
local storage — reads are local on every node, so any node authenticates.

Enablement follows the etcd model: deploy with auth.enabled=false, create the
identities (`consensusdb iam bootstrap`, `iam user add`, `iam sa add`), then
enable and restart. With auth.enabled=true every connection must authenticate;
reconnects re-present the credential automatically (value-rpc handshake).
*/
type AuthService struct {
	Enabled bool            `value:"auth.enabled,default=false"`
	Storage KeyValueStorage `inject:""`
	Log     *zap.Logger     `inject:""`
}

func (t *AuthService) BeanName() string { return "auth-service" }

// Authenticate is the valueserver.Authenticator installed on the data plane.
func (t *AuthService) Authenticate(conn valuerpc.MsgConn, credential value.Value) (string, error) {
	if m, ok := credential.(value.Map); ok {
		switch method := m.GetString("method").String(); method {
		case "password":
			return t.passwordPrincipal(conn, m)
		case "token":
			return t.tokenPrincipal(conn, m)
		default:
			return "", xerrors.Errorf("unsupported credential method %q", method)
		}
	}
	// No explicit credential: fall back to the verified client certificate.
	if principal, ok := t.certificatePrincipal(conn); ok {
		return principal, nil
	}
	t.deny(conn, "", "no credential and no registered client certificate")
	return "", xerrors.New("authentication required")
}

func (t *AuthService) passwordPrincipal(conn valuerpc.MsgConn, m value.Map) (string, error) {
	name := m.GetString("user").String()
	pass := m.GetString("pass").String()
	rec := &iam.UserRecord{}
	if !t.load(iam.UserPrefix+name, rec) || rec.Disabled || !iam.VerifyPassword(rec.PasswordHash, pass) {
		t.deny(conn, name, "invalid user credentials")
		return "", xerrors.New("invalid user credentials")
	}
	return iam.PrincipalUser(name), nil
}

func (t *AuthService) tokenPrincipal(conn valuerpc.MsgConn, m value.Map) (string, error) {
	name, secret, ok := iam.ParseToken(m.GetString("token").String())
	if !ok {
		t.deny(conn, "", "malformed token")
		return "", xerrors.New("invalid token")
	}
	rec := &iam.ServiceAccountRecord{}
	if !t.load(iam.ServiceAccountPrefix+name, rec) || rec.Disabled ||
		rec.TokenHash == "" || !iam.VerifyTokenSecret(rec.TokenHash, secret) {
		t.deny(conn, name, "invalid token")
		return "", xerrors.New("invalid token")
	}
	return iam.PrincipalServiceAccount(name), nil
}

// certificatePrincipal maps the verified peer certificate onto a registered
// service account: SAN URIs first (the recommended identity form), then the CN.
func (t *AuthService) certificatePrincipal(conn valuerpc.MsgConn) (string, bool) {
	certs, ok := valuerpc.PeerCertificates(conn)
	if !ok || len(certs) == 0 {
		return "", false
	}
	leaf := certs[0]
	idents := make([]string, 0, len(leaf.URIs)+1)
	for _, uri := range leaf.URIs {
		idents = append(idents, uri.String())
	}
	if cn := leaf.Subject.CommonName; cn != "" {
		idents = append(idents, cn)
	}
	for _, ident := range idents {
		idx := &iam.CertIndexRecord{}
		if !t.load(iam.CertPrefix+ident, idx) {
			continue
		}
		rec := &iam.ServiceAccountRecord{}
		if !t.load(iam.ServiceAccountPrefix+idx.ServiceAccount, rec) || rec.Disabled {
			continue
		}
		return iam.PrincipalServiceAccount(rec.Name), true
	}
	return "", false
}

// load reads and decodes one IAM record from local storage; false when missing
// or undecodable.
func (t *AuthService) load(minor string, obj interface{}) bool {
	rec, err := t.Storage.Get(&pb.KeyRequest{Key: iam.Key(minor)})
	if err != nil || rec == nil || len(rec.Value) == 0 {
		return false
	}
	if err := iam.Decode(rec.Value, obj); err != nil {
		t.Log.Warn("IamRecordDecode", zap.String("minor", minor), zap.Error(err))
		return false
	}
	return true
}

// deny logs a failed authentication (name may be empty). This is the seed of the
// access audit trail; the structured audit stream arrives with authorization.
func (t *AuthService) deny(conn valuerpc.MsgConn, name, reason string) {
	remote := ""
	if conn != nil {
		remote = conn.RemoteAddr()
	}
	t.Log.Warn("AuthDenied", zap.String("name", name), zap.String("reason", reason), zap.String("remote", remote))
}
