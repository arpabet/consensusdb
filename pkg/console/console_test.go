/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
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
