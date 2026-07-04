/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/consensusdb/pkg/verify"
	"go.arpabet.com/raft/raftapi"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.uber.org/zap"
)

// LedgerHead is the chain-head accessor the API surfaces (satisfied by the FSM).
type LedgerHead interface {
	ChainHead() (uint64, [32]byte)
}

/*
ConsoleHandler serves the admin web console's REST API under /api/. Every request
authenticates (Bearer token or HTTP Basic against IAM) and is authorized by
permission; the console is read-and-verify, so it needs cdb.proofs.read. The
heavy operation — verifying a backup against a quorum certificate — runs as a
background job the UI polls for progress.
*/
type ConsoleHandler struct {
	Auth   *server.AuthService   `inject:""`
	Policy *server.PolicyService `inject:"optional"`
	Jobs   *JobManager           `inject:""`
	Head   LedgerHead            `inject:"optional"` // the FSM
	Raft   raftapi.RaftServer    `inject:"optional"`
	Log    *zap.Logger           `inject:""`
}

func (t *ConsoleHandler) BeanName() string { return "console-handler" }

// Pattern is a gorilla-mux catch-all so every /api/* path reaches this handler.
func (t *ConsoleHandler) Pattern() string { return "/api/{rest:.*}" }

func (t *ConsoleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := t.authenticate(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	// Authorize with the console's read/verify permission.
	ctx := valuerpc.ContextWithPrincipal(r.Context(), principal)
	if err := t.Policy.Authorize(ctx, iam.PermProofsRead, "", ""); err != nil {
		writeErr(w, http.StatusForbidden, "permission denied")
		return
	}

	switch path := r.URL.Path; {
	case path == "/api/cluster" && r.Method == http.MethodGet:
		t.cluster(w)
	case path == "/api/ledger/status" && r.Method == http.MethodGet:
		t.ledgerStatus(w)
	case path == "/api/ledger/verify" && r.Method == http.MethodPost:
		t.startVerify(w, r)
	case path == "/api/ledger/verify" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"jobs": t.Jobs.List()})
	case strings.HasPrefix(path, "/api/ledger/verify/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(path, "/api/ledger/verify/")
		if job, ok := t.Jobs.Get(id); ok {
			writeJSON(w, http.StatusOK, job)
		} else {
			writeErr(w, http.StatusNotFound, "no such job")
		}
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// authenticate resolves the request principal from a Bearer token or HTTP Basic
// credentials (validated against IAM).
func (t *ConsoleHandler) authenticate(r *http.Request) (string, bool) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if p, err := t.Auth.AuthenticateToken(strings.TrimPrefix(auth, "Bearer ")); err == nil {
			return p, true
		}
		return "", false
	}
	if user, pass, ok := r.BasicAuth(); ok {
		if p, err := t.Auth.AuthenticatePassword(user, pass); err == nil {
			return p, true
		}
	}
	return "", false
}

func (t *ConsoleHandler) cluster(w http.ResponseWriter) {
	out := map[string]any{"replication": false}
	if t.Raft != nil {
		if r, ok := t.Raft.Raft(); ok {
			out["replication"] = true
			out["state"] = r.State().String()
			out["leader"] = string(r.Leader())
			out["appliedIndex"] = r.AppliedIndex()
			out["lastIndex"] = r.LastIndex()
			if stats := r.Stats(); stats != nil {
				out["term"] = stats["term"]
				out["numPeers"] = stats["num_peers"]
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (t *ConsoleHandler) ledgerStatus(w http.ResponseWriter) {
	if t.Head == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	index, digest := t.Head.ChainHead()
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"height":    index,
		"digest":    ledger.DigestHex(digest),
	})
}

// verifyRequest is the POST /api/ledger/verify body. Byte fields are base64.
type verifyRequest struct {
	Source     string   `json:"source"`     // backup path or s3://bucket/key
	Password   string   `json:"password"`   // dump password (optional)
	CACert     string   `json:"caCert"`     // base64 ledger CA public key
	QuorumCert string   `json:"quorumCert"` // base64 quorum certificate
	NodeCerts  []string `json:"nodeCerts"`  // base64 node certs
	Threshold  int      `json:"threshold"`
	S3         struct {
		Endpoint  string `json:"endpoint"`
		Region    string `json:"region"`
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
		UseSSL    bool   `json:"useSSL"`
	} `json:"s3"`
}

func (t *ConsoleHandler) startVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	caCert, err1 := base64.StdEncoding.DecodeString(req.CACert)
	quorum, err2 := base64.StdEncoding.DecodeString(req.QuorumCert)
	if err1 != nil || err2 != nil || req.Source == "" {
		writeErr(w, http.StatusBadRequest, "source, caCert and quorumCert are required")
		return
	}
	var nodeCerts [][]byte
	for _, nc := range req.NodeCerts {
		raw, err := base64.StdEncoding.DecodeString(nc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid node cert encoding")
			return
		}
		nodeCerts = append(nodeCerts, raw)
	}
	id := t.Jobs.StartVerifyBackup(verify.Options{
		Source:     req.Source,
		Password:   req.Password,
		CACert:     caCert,
		QuorumCert: quorum,
		NodeCerts:  nodeCerts,
		Threshold:  req.Threshold,
		S3: backup.S3Config{
			Endpoint:  req.S3.Endpoint,
			Region:    req.S3.Region,
			AccessKey: req.S3.AccessKey,
			SecretKey: req.S3.SecretKey,
			UseSSL:    req.S3.UseSSL,
		},
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
