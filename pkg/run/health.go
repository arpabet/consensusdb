/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"net/http"
	"strings"
)

/*
HealthHandler answers a fixed path with a plain-text "OK" (HTTP 200) — the simple
shape Kubernetes HTTP liveness/readiness probes expect (they key on the status
code; the body is a human-friendly confirmation). Register one per path, e.g.
/healthz, /livez, /readyz.

This is a liveness signal: if the process can serve HTTP it is alive. It does not
gate on cluster readiness, so it will not flap a pod out of rotation during a
transient raft election.
*/
type HealthHandler struct {
	pattern string
}

// NewHealthHandler builds a health endpoint at the given path (e.g. "/healthz").
func NewHealthHandler(pattern string) *HealthHandler { return &HealthHandler{pattern: pattern} }

func (t *HealthHandler) BeanName() string {
	return "health" + strings.ReplaceAll(t.pattern, "/", "-")
}

func (t *HealthHandler) Pattern() string { return t.pattern }

func (t *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte("OK"))
	}
}
