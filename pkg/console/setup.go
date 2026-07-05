/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
)

/*
First-run onboarding, database export/import, and ledger-CA generation for the
web console. The onboarding endpoints are unauthenticated because on a fresh
cluster no identity exists yet; bootstrap self-guards on the absence of any admin
user so it cannot be used to escalate once setup is done.
*/

// setupStatus reports whether the cluster still needs first-run setup (no user
// identity exists yet) and whether authentication is enforced.
func (t *ConsoleHandler) setupStatus(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"needsSetup":  !t.anyUserExists(),
		"authEnabled": t.Auth != nil && t.Auth.Enabled,
	})
}

// setupBootstrap creates the initial admin user. It succeeds only while no user
// exists (create-if-absent on the record plus an up-front scan), so it is safe to
// leave unauthenticated on a fresh cluster and inert afterwards.
func (t *ConsoleHandler) setupBootstrap(w http.ResponseWriter, r *http.Request) {
	if t.anyUserExists() {
		writeErr(w, http.StatusForbidden, "setup already completed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil ||
		req.Username == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "username and a password (min 8 chars) are required")
		return
	}
	hash, err := iam.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash password")
		return
	}
	raw, err := iam.Encode(&iam.UserRecord{
		Name: req.Username, PasswordHash: hash, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode record")
		return
	}
	// Create-if-absent (CAS version 0) so concurrent bootstraps can't both win.
	status, err := t.svc.Put(context.Background(), &pb.RecordRequest{
		Key: iam.Key(iam.UserPrefix + req.Username), Value: raw, CompareAndSet: true, Version: 0,
	})
	if err != nil || status == nil || !status.Updated {
		writeErr(w, http.StatusConflict, "admin user already exists")
		return
	}
	// The first user is the administrator: bind roles/cdb.admin at instance scope
	// (all tenants & regions). Admin-ness is this role, not a separate flag.
	if err := t.grantRole([]string{iam.PrincipalUser(req.Username)}, iam.RoleAdmin, "", "", true); err != nil {
		writeErr(w, http.StatusInternalServerError, "grant admin role")
		return
	}
	// Mint the built-in CA now so the instance can issue client and node
	// certificates. Non-fatal: it is also created lazily on first issuance.
	if _, err := t.ensureCA(context.Background()); err != nil {
		t.Log.Warn("PkiEnsureCA", zap.Error(err))
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"created":     req.Username,
		"authEnabled": t.Auth != nil && t.Auth.Enabled,
		"note":        "set AUTH_ENABLED=true and restart the nodes to enforce authentication",
	})
}

// anyUserExists scans the IAM region for any user record.
func (t *ConsoleHandler) anyUserExists() bool {
	found := false
	sender := senderFunc(func(block *pb.Block) error {
		for _, rec := range block.Record {
			if rec != nil && rec.Key != nil && strings.HasPrefix(string(rec.Key.MinorKey), iam.UserPrefix) {
				found = true
			}
		}
		return nil
	})
	_ = t.Storage.GetArea(&pb.KeyRequest{Key: &pb.Key{
		MajorKey: []byte(iam.SystemTenant), RegionName: []byte(iam.Region),
	}}, server.RegionNameField, sender)
	return found
}

// exportDatabase streams a whole-store dump as a download, optionally encrypted
// with ?password=… (argon2id + AES-256-GCM), so an admin can save a backup from
// the browser.
func (t *ConsoleHandler) exportDatabase(w http.ResponseWriter, r *http.Request) {
	password := r.URL.Query().Get("password")
	name := "consensusdb-" + time.Now().UTC().Format("20060102T150405Z") + ".dump"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")

	container, err := backup.NewWriter(w, password)
	if err != nil {
		t.Log.Error("ExportInit", zap.Error(err))
		return
	}
	if _, err := t.Storage.Backup(container, 0); err != nil {
		t.Log.Error("Export", zap.Error(err)) // headers already sent; log and stop
		return
	}
	_ = container.Close()
}

// importDatabase loads an uploaded dump into local storage. It is refused while
// replication is active (a raft cluster would diverge) — import into a fresh node.
// The dump password is taken from ?password=… .
func (t *ConsoleHandler) importDatabase(w http.ResponseWriter, r *http.Request) {
	if t.Replicator != nil && t.Replicator.Enabled() {
		writeErr(w, http.StatusConflict, "import is disabled while replication is active: import into a fresh node, then bootstrap")
		return
	}
	reader, err := backup.NewReader(r.Body, r.URL.Query().Get("password"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := t.Storage.Load(reader); err != nil {
		writeErr(w, http.StatusBadRequest, "load dump: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "imported"})
}

// generateLedgerCA mints a ledger CA and returns the key material (base64) for
// download — a convenience for the onboarding wizard. For production the CA
// private key should be generated and kept offline via `consensusdb ledger
// ca-init`; this endpoint requires cluster-admin.
func (t *ConsoleHandler) generateLedgerCA(w http.ResponseWriter) {
	ca, err := ledger.GenerateCA()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "generate CA")
		return
	}
	seed, _ := ca.MarshalBinary()
	pub, _ := ca.Public().MarshalBinary()
	writeJSON(w, http.StatusOK, map[string]string{
		"caKey": base64.StdEncoding.EncodeToString(seed),
		"caPub": base64.StdEncoding.EncodeToString(pub),
		"note":  "store caKey offline; distribute caPub to verifiers",
	})
}

// senderFunc adapts a function to server.BlockSender.
type senderFunc func(*pb.Block) error

func (f senderFunc) Send(b *pb.Block) error { return f(b) }
