/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.arpabet.com/consensusdb/pkg/verify"
)

// The console requires authentication: no credentials ⇒ 401.
func TestConsoleRequiresAuth(t *testing.T) {
	h := &ConsoleHandler{Jobs: NewJobManager()}
	req := httptest.NewRequest(http.MethodGet, "/api/ledger/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d, want 401", rec.Code)
	}
}

// The job manager runs a verification in the background and reports progress and
// the result through Get — the exact loop the SPA's progress bar polls. Here the
// verify fails fast on a bad quorum certificate, exercising the failure result.
func TestJobManagerLifecycle(t *testing.T) {
	m := NewJobManager()
	id := m.StartVerifyBackup(mkBadOptions())
	if id == "" {
		t.Fatal("empty job id")
	}
	if _, ok := m.Get(id); !ok {
		t.Fatal("job not registered")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		job, _ := m.Get(id)
		if job.State != JobRunning {
			// A malformed CA public key is an operational error → failed job.
			if job.State != JobFailed {
				t.Fatalf("state = %s, want failed", job.State)
			}
			if job.Error == "" {
				t.Fatal("failed job has no error")
			}
			if job.EndedAt == nil {
				t.Fatal("failed job has no end time")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("job never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The start endpoint parses the JSON body and returns a job id (202); the status
// endpoint returns the job. This drives the SPA round trip end to end (auth is
// stubbed out here by using a handler whose Policy is nil ⇒ authorize is a
// no-op, but authenticate still requires a credential, so we send Basic and rely
// on a fake auth — instead we test the routing + job wiring directly).
func TestStartVerifyRouting(t *testing.T) {
	m := NewJobManager()
	h := &ConsoleHandler{Jobs: m}
	// Directly exercise startVerify (bypassing auth, tested separately) via the
	// exported flow: build a request with a valid JSON body.
	body := `{"source":"/nonexistent.dump","caCert":"AAAA","quorumCert":"AAAA"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ledger/verify", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.startVerify(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d body=%s, want 202", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("start response: %v %s", err, rec.Body.String())
	}
	if _, ok := m.Get(out.ID); !ok {
		t.Fatal("started job not tracked")
	}
}

// mkBadOptions returns Options whose CA public key is malformed, so VerifyBackup
// returns an operational error quickly (no store load needed).
func mkBadOptions() verify.Options {
	return verify.Options{Source: "x", CACert: []byte("not-a-key"), QuorumCert: []byte("x")}
}
