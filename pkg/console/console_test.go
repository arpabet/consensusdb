/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/replication"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

func newConsole(t *testing.T) (*ConsoleHandler, server.KeyValueStorage) {
	t.Helper()
	conf := &server.Configuration{DataDir: t.TempDir(), FileIO: true}
	if err := conf.PostConstruct(); err != nil {
		t.Fatal(err)
	}
	storage, err := server.OpenKeyValueStorage(conf, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	h := &ConsoleHandler{
		Auth:    &server.AuthService{Storage: storage, Log: zap.NewNop()},
		Storage: storage,
		Jobs:    NewJobManager(),
		Log:     zap.NewNop(),
		DataDir: conf.DataDir, // ensureCA looks here for a staged genesis CA
		// Policy nil ⇒ Authorize is a no-op (auth disabled); this test focuses on
		// the routing, onboarding self-guard, and data operations.
	}
	if err := h.PostConstruct(); err != nil {
		t.Fatal(err)
	}
	return h, storage
}

func do(h *ConsoleHandler, method, path string, body []byte) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// First-run onboarding: setup is needed, bootstrap creates the first admin, and
// a second bootstrap is refused (the self-guard that makes the unauthenticated
// endpoint safe once setup is done).
func TestOnboardingBootstrap(t *testing.T) {
	h, storage := newConsole(t)

	rec := do(h, http.MethodGet, "/api/setup/status", nil)
	var st struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.NeedsSetup {
		t.Fatal("fresh cluster must need setup")
	}

	rec = do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap = %d %s", rec.Code, rec.Body.String())
	}
	// The admin now exists and authenticates.
	if _, err := h.Auth.AuthenticatePassword("root", "supersecret"); err != nil {
		t.Fatalf("created admin cannot log in: %v", err)
	}
	// Setup no longer needed; a second bootstrap is refused.
	rec = do(h, http.MethodGet, "/api/setup/status", nil)
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.NeedsSetup {
		t.Fatal("setup must be complete after bootstrap")
	}
	rec = do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"evil","password":"supersecret"}`))
	if rec.Code == http.StatusCreated {
		t.Fatal("a second bootstrap must be refused")
	}

	// Bootstrap also mints the single built-in CA, and ensureCA is idempotent:
	// a second call returns the same root rather than a new one.
	ca, ok, err := h.loadCA(context.Background())
	if err != nil || !ok {
		t.Fatalf("CA not created by bootstrap: ok=%v err=%v", ok, err)
	}
	again, err := h.ensureCA(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ca.Cert.SerialNumber.Cmp(again.Cert.SerialNumber) != 0 {
		t.Fatal("ensureCA minted a second CA instead of reusing the first")
	}
	_ = storage
}

// Export streams a dump that Import loads back, and the round trip preserves data.
func TestExportImportRoundTrip(t *testing.T) {
	h, storage := newConsole(t)
	// seed a record
	key := &pb.Key{MajorKey: []byte("acme"), RegionName: []byte("USERS"), MinorKey: []byte("alice")}
	if _, err := storage.Put(&pb.RecordRequest{Key: key, Value: []byte("hello")}, 1); err != nil {
		t.Fatal(err)
	}

	rec := do(h, http.MethodGet, "/api/database/export?password=pw", nil)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("export = %d len=%d", rec.Code, rec.Body.Len())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Fatal("export missing download header")
	}
	dump := rec.Body.Bytes()

	// import into a FRESH console/store
	h2, storage2 := newConsole(t)
	rec = do(h2, http.MethodPost, "/api/database/import?password=pw", dump)
	if rec.Code != http.StatusOK {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	got, err := storage2.Get(&pb.KeyRequest{Key: key})
	if err != nil || got == nil || string(got.Value) != "hello" {
		t.Fatalf("imported record = %v err=%v, want hello", got, err)
	}
	// a wrong password must fail the import
	h3, _ := newConsole(t)
	rec = do(h3, http.MethodPost, "/api/database/import?password=wrong", dump)
	if rec.Code == http.StatusOK {
		t.Fatal("import with wrong password must fail")
	}
}

// The regions dashboard reports each (tenant, region) with key counts and sizes.
func TestRegionsDashboard(t *testing.T) {
	h, storage := newConsole(t)
	seed := map[string][]string{"USERS": {"a", "b", "c"}, "JOBS": {"j1"}}
	for region, keys := range seed {
		for _, k := range keys {
			key := &pb.Key{MajorKey: []byte("acme"), RegionName: []byte(region), MinorKey: []byte(k)}
			if _, err := storage.Put(&pb.RecordRequest{Key: key, Value: bytes.Repeat([]byte("x"), 100)}, 1); err != nil {
				t.Fatal(err)
			}
		}
	}
	rec := do(h, http.MethodGet, "/api/regions", nil)
	var out struct {
		Regions []RegionStat `json:"regions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	byName := map[string]RegionStat{}
	for _, r := range out.Regions {
		byName[r.Region] = r
	}
	if byName["USERS"].Keys != 3 || byName["JOBS"].Keys != 1 {
		t.Fatalf("region key counts wrong: %+v", out.Regions)
	}
	if byName["USERS"].TransferByte < 300 {
		t.Fatalf("USERS transfer bytes = %d, want ≥300", byName["USERS"].TransferByte)
	}

	// stats endpoint returns cumulative counters + disk size.
	rec = do(h, http.MethodGet, "/api/stats", nil)
	var stats map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if _, ok := stats["writes"]; !ok {
		t.Fatal("stats missing writes counter")
	}
	if _, ok := stats["diskBytes"]; !ok {
		t.Fatal("stats missing diskBytes")
	}
}

// Onboarding-created identity gets the admin flag (so the UI can gate on it).
func TestBootstrapSetsAdmin(t *testing.T) {
	h, storage := newConsole(t)
	do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`))
	if rec, err := storage.Get(&pb.KeyRequest{Key: iam.Key(iam.UserPrefix + "root")}); err != nil || rec == nil {
		t.Fatal("root user not stored")
	}
	// Admin-ness is a roles/cdb.admin binding at instance scope, not a flag.
	rec, err := storage.Get(&pb.KeyRequest{Key: iam.Key(iam.PolicyInstance)})
	if err != nil || rec == nil {
		t.Fatal("instance policy not stored")
	}
	p := &iam.PolicyRecord{}
	if err := iam.Decode(rec.Value, p); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	found := false
	for _, b := range p.Bindings {
		if b.Role != iam.RoleAdmin {
			continue
		}
		for _, m := range b.Members {
			if m == iam.PrincipalUser("root") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("bootstrap must bind %s to user:root at instance; got %+v", iam.RoleAdmin, p.Bindings)
	}
}

// A client certificate for a user is issued by the built-in CA (chaining to it,
// carrying the principal's SAN URI), listed, and revoked; an external identity can
// be registered alongside it.
func TestClientCertIssueRegisterRevoke(t *testing.T) {
	h, _ := newConsole(t)
	if rec := do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodPost, "/api/iam/users", []byte(`{"username":"alice","password":"alicesecret"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("create user = %d %s", rec.Code, rec.Body.String())
	}

	// Issue a CA-signed client cert for user:alice.
	rec := do(h, http.MethodPost, "/api/iam/certs/issue", []byte(`{"principal":"user:alice","ttlDays":30}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue cert = %d %s", rec.Code, rec.Body.String())
	}
	var iss struct{ Identity, CertPem, KeyPem, CaPem string }
	if err := json.Unmarshal(rec.Body.Bytes(), &iss); err != nil {
		t.Fatal(err)
	}
	if iss.Identity != "cdb://user/alice" {
		t.Fatalf("identity = %q", iss.Identity)
	}
	// The leaf chains to the returned CA and carries the principal's SAN URI.
	caB, _ := pem.Decode([]byte(iss.CaPem))
	caCert, err := x509.ParseCertificate(caB.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	leafB, _ := pem.Decode([]byte(iss.CertPem))
	leaf, err := x509.ParseCertificate(leafB.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("issued cert does not chain to CA: %v", err)
	}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != "cdb://user/alice" {
		t.Fatalf("SAN URIs = %v", leaf.URIs)
	}
	if _, err := tls.X509KeyPair([]byte(iss.CertPem), []byte(iss.KeyPem)); err != nil {
		t.Fatalf("returned key does not match cert: %v", err)
	}

	// Register an external identity for the same principal.
	if rec := do(h, http.MethodPost, "/api/iam/certs/register", []byte(`{"principal":"user:alice","identity":"CN=alice-laptop"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("register cert = %d %s", rec.Code, rec.Body.String())
	}
	// Registering for a principal that does not exist is refused.
	if rec := do(h, http.MethodPost, "/api/iam/certs/register", []byte(`{"principal":"user:ghost","identity":"CN=ghost"}`)); rec.Code != http.StatusBadRequest {
		t.Fatalf("register for missing principal = %d, want 400", rec.Code)
	}

	// List shows both, the issued one flagged.
	rec = do(h, http.MethodGet, "/api/iam/certs?principal="+url.QueryEscape("user:alice"), nil)
	var listed struct {
		Certs []struct {
			Identity string `json:"identity"`
			Issued   bool   `json:"issued"`
		} `json:"certs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Certs) != 2 {
		t.Fatalf("listed %d certs, want 2: %s", len(listed.Certs), rec.Body.String())
	}
	issuedSeen := false
	for _, c := range listed.Certs {
		if c.Identity == "cdb://user/alice" && c.Issued {
			issuedSeen = true
		}
	}
	if !issuedSeen {
		t.Fatal("issued cert not flagged in list")
	}

	// Revoke the registered identity; only the issued one remains.
	if rec := do(h, http.MethodDelete, "/api/iam/certs?identity="+url.QueryEscape("CN=alice-laptop"), nil); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(h, http.MethodGet, "/api/iam/certs?principal="+url.QueryEscape("user:alice"), nil)
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Certs) != 1 || listed.Certs[0].Identity != "cdb://user/alice" {
		t.Fatalf("after revoke, certs = %s", rec.Body.String())
	}
}

// nodeCSR builds a PEM CSR for a joining node's fresh key.
func nodeCSR(t *testing.T, cn string) []byte {
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

// A join token authorizes signing a node certificate (server+client, chaining to
// the CA, with the node id and address as SANs); it is single-use and honours
// expiry.
func TestJoinTokenNodeEnrollment(t *testing.T) {
	h, _ := newConsole(t)
	if rec := do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap = %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()

	token, exp, err := h.mintJoinToken(ctx, time.Hour, "user:root")
	if err != nil {
		t.Fatal(err)
	}
	if token[:5] != "join-" || exp == 0 {
		t.Fatalf("bad join token %q exp=%d", token, exp)
	}

	certPEM, caPEM, err := h.signNodeEnrollment(ctx, token, "node-2", []string{"10.0.0.2"}, nodeCSR(t, "node-2"))
	if err != nil {
		t.Fatalf("sign enrollment: %v", err)
	}
	caB, _ := pem.Decode(caPEM)
	caCert, _ := x509.ParseCertificate(caB.Bytes)
	leafB, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(leafB.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	// A node cert must validate for BOTH server and client auth (peers dial each other).
	for _, ku := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{ku}}); err != nil {
			t.Fatalf("node cert not valid for %v: %v", ku, err)
		}
	}
	if leaf.Subject.CommonName != "node-2" {
		t.Fatalf("CN = %q", leaf.Subject.CommonName)
	}
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "10.0.0.2" {
		t.Fatalf("IP SANs = %v", leaf.IPAddresses)
	}

	// Single-use is enforced at signing: the token is burned atomically BEFORE the
	// cert is issued, so a second enrollment is rejected even without the
	// post-join cleanup.
	if _, _, err := h.signNodeEnrollment(ctx, token, "node-3", []string{"10.0.0.3"}, nodeCSR(t, "node-3")); err == nil {
		t.Fatal("a join token signed two enrollments")
	}
	h.consumeJoinToken(ctx, token) // tidy-up stays idempotent
	if _, _, err := h.signNodeEnrollment(ctx, token, "node-2", []string{"10.0.0.2"}, nodeCSR(t, "node-2")); err == nil {
		t.Fatal("consumed join token still signed a cert")
	}
	// The node cert carries the cluster-wide SAN peers verify against.
	if !contains(leaf.DNSNames, iam.NodeSANDNS) {
		t.Fatalf("node cert DNS SANs = %v, want %s present", leaf.DNSNames, iam.NodeSANDNS)
	}

	// An expired token is refused.
	expToken, expHash, _ := iam.NewToken(iam.TokenPrefixJoin)
	raw, _ := iam.Encode(&iam.JoinRecord{ExpiresAt: time.Now().Add(-time.Minute).Unix()})
	if _, err := h.svc.Put(ctx, &pb.RecordRequest{Key: iam.PKIKey(iam.JoinIndexKey(expHash)), Value: raw}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.signNodeEnrollment(ctx, expToken, "node-9", []string{"10.0.0.9"}, nodeCSR(t, "node-9")); err == nil {
		t.Fatal("expired join token accepted")
	}
}

// A user PAT is minted (shown once), authenticates as the user, is listed and
// revoked, and expired PATs are rejected.
func TestUserPAT(t *testing.T) {
	h, _ := newConsole(t)
	do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`))
	if rec := do(h, http.MethodPost, "/api/iam/users", []byte(`{"username":"alice","password":"alicesecret"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("create user = %d %s", rec.Code, rec.Body.String())
	}

	// Mint a PAT for alice.
	rec := do(h, http.MethodPost, "/api/iam/users/alice/tokens", []byte(`{"label":"laptop","ttlDays":30}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint PAT = %d %s", rec.Code, rec.Body.String())
	}
	var pat struct{ ID, Token, Label string }
	json.Unmarshal(rec.Body.Bytes(), &pat)
	if pat.Token[:4] != "pat-" || pat.Label != "laptop" {
		t.Fatalf("bad PAT response: %s", rec.Body.String())
	}
	// The PAT authenticates as the user.
	if p, err := h.Auth.AuthenticateToken(pat.Token); err != nil || p != iam.PrincipalUser("alice") {
		t.Fatalf("PAT auth = %q err=%v, want user:alice", p, err)
	}

	// It appears in the user's token list with its label.
	rec = do(h, http.MethodGet, "/api/iam/users/alice/tokens", nil)
	var listed struct {
		Tokens []struct {
			ID, Label string
			ExpiresAt int64
		} `json:"tokens"`
	}
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tokens) != 1 || listed.Tokens[0].Label != "laptop" || listed.Tokens[0].ExpiresAt == 0 {
		t.Fatalf("PAT list = %s", rec.Body.String())
	}

	// Revoke it — auth stops working and the list empties.
	if rec := do(h, http.MethodDelete, "/api/iam/users/alice/tokens/"+pat.ID, nil); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := h.Auth.AuthenticateToken(pat.Token); err == nil {
		t.Fatal("revoked PAT still authenticates")
	}
	rec = do(h, http.MethodGet, "/api/iam/users/alice/tokens", nil)
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tokens) != 0 {
		t.Fatalf("PAT list not empty after revoke: %s", rec.Body.String())
	}

	// An expired PAT is rejected by authentication.
	expTok, expHash, _ := iam.NewToken(iam.TokenPrefixUser)
	raw, _ := iam.Encode(&iam.TokenRecord{Principal: iam.PrincipalUser("alice"), ExpiresAt: time.Now().Add(-time.Hour).Unix(), Label: "old"})
	h.svc.Put(context.Background(), &pb.RecordRequest{Key: iam.Key(iam.TokenIndexKey(expHash)), Value: raw})
	if _, err := h.Auth.AuthenticateToken(expTok); err == nil {
		t.Fatal("expired PAT authenticated")
	}
}

// Deleting a user revokes everything that could keep the identity alive: PATs,
// cert identities, role bindings, and group memberships.
func TestDeleteUserPurgesCredentials(t *testing.T) {
	h, _ := newConsole(t)
	do(h, http.MethodPost, "/api/setup/bootstrap", []byte(`{"username":"root","password":"supersecret"}`))
	do(h, http.MethodPost, "/api/iam/users", []byte(`{"username":"mallory","password":"mallorypwd"}`))

	// Arm the identity: a PAT, a cert identity, a role binding, a group membership.
	rec := do(h, http.MethodPost, "/api/iam/users/mallory/tokens", []byte(`{"label":"x","ttlDays":30}`))
	var pat struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &pat)
	if rec := do(h, http.MethodPost, "/api/iam/certs/register", []byte(`{"principal":"user:mallory","identity":"CN=mallory"}`)); rec.Code != http.StatusCreated {
		t.Fatalf("register cert = %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/iam/bindings", []byte(`{"role":"roles/cdb.editor","members":["user:mallory"]}`)); rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("bind = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodPost, "/api/iam/groups", []byte(`{"name":"team","members":["user:mallory","user:root"]}`)); rec.Code >= 300 {
		t.Fatalf("group = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := h.Auth.AuthenticateToken(pat.Token); err != nil {
		t.Fatalf("PAT must work before delete: %v", err)
	}

	if rec := do(h, http.MethodDelete, "/api/iam/users/mallory", nil); rec.Code != http.StatusOK {
		t.Fatalf("delete user = %d %s", rec.Code, rec.Body.String())
	}

	// The PAT no longer authenticates.
	if _, err := h.Auth.AuthenticateToken(pat.Token); err == nil {
		t.Fatal("deleted user's PAT still authenticates")
	}
	// The cert identity is gone.
	rec = do(h, http.MethodGet, "/api/iam/certs?principal="+url.QueryEscape("user:mallory"), nil)
	var certs struct {
		Certs []any `json:"certs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &certs)
	if len(certs.Certs) != 0 {
		t.Fatalf("deleted user's cert identities remain: %s", rec.Body.String())
	}
	// No binding anywhere still names the principal.
	rec = do(h, http.MethodGet, "/api/iam/bindings", nil)
	if strings.Contains(rec.Body.String(), "user:mallory") {
		t.Fatalf("deleted user still bound: %s", rec.Body.String())
	}
	// The group no longer lists the member (but survives with other members).
	rec = do(h, http.MethodGet, "/api/iam/groups", nil)
	if strings.Contains(rec.Body.String(), "user:mallory") || !strings.Contains(rec.Body.String(), "user:root") {
		t.Fatalf("group membership not cleaned: %s", rec.Body.String())
	}
}

// In cluster mode the seed stages its genesis CA on disk; ensureCA must adopt
// that root (never mint a second one), and a node that trusts a transport CA it
// cannot sign for must refuse rather than fork the PKI.
func TestEnsureCAAdoptsGenesis(t *testing.T) {
	h, _ := newConsole(t)
	ctx := context.Background()

	// Stage a genesis CA + identity in the console's data dir, as NodeTLSFactory
	// does on the seed.
	id, caRec, err := replication.GenesisIdentity("node-1", []string{"10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := id.Save(h.DataDir); err != nil {
		t.Fatal(err)
	}
	if err := replication.SaveGenesisCA(h.DataDir, caRec); err != nil {
		t.Fatal(err)
	}

	ca, err := h.ensureCA(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staged, _ := caRec.Load()
	if ca.Cert.SerialNumber.Cmp(staged.Cert.SerialNumber) != 0 {
		t.Fatal("ensureCA minted a new CA instead of adopting the staged genesis root")
	}

	// A joiner: trusts the transport CA (ca.pem) but has no signing key and no
	// published record → must refuse, not fork.
	h2, _ := newConsole(t)
	if err := (&replication.NodeIdentity{CertPEM: id.CertPEM, KeyPEM: id.KeyPEM, CAPEM: id.CAPEM}).Save(h2.DataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := h2.ensureCA(context.Background()); err == nil {
		t.Fatal("joiner without the CA key minted a second root")
	}
}
