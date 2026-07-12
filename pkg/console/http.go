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
	Auth       *server.AuthService    `inject:""`
	Policy     *server.PolicyService  `inject:"optional"`
	Storage    server.KeyValueStorage `inject:""`
	Replicator server.Replicator      `inject:"optional"`
	Jobs       *JobManager            `inject:""`
	Head       LedgerHead             `inject:"optional"` // the FSM
	Raft       raftapi.RaftServer     `inject:"optional"`
	Log        *zap.Logger            `inject:""`

	DataDir  string `value:"consensusdb.data-dir,default=/tmp/consensusdb"`
	HTTPBind string `value:"http-server.bind-address,default=0.0.0.0:8441"`
	// BootstrapToken is the pre-shared cluster formation secret (env
	// CONSENSUSDB_BOOTSTRAP_TOKEN; on Kubernetes one Secret shared by every pod).
	// When a joining node presents it, the leader adopts it as a reusable join
	// record (adoptBootstrapToken), so every fresh ordinal enrolls with the same
	// secret and cluster formation needs no per-node token minting. Empty
	// disables the fallback; minted single-use join tokens work regardless.
	BootstrapToken string `value:"consensusdb.bootstrap-token,default="`

	svc          *server.KeyValueService // routes IAM writes through raft when enabled
	regionsCache regionsCache
}

func (t *ConsoleHandler) BeanName() string { return "console-handler" }

func (t *ConsoleHandler) PostConstruct() error {
	t.svc = &server.KeyValueService{Storage: t.Storage, Replicator: t.Replicator, Log: t.Log}
	return nil
}

// Pattern is a gorilla-mux catch-all so every /api/* path reaches this handler.
func (t *ConsoleHandler) Pattern() string { return "/api/{rest:.*}" }

func (t *ConsoleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, method := r.URL.Path, r.Method

	// First-run onboarding endpoints are unauthenticated (no identities exist
	// yet); bootstrap self-guards on there being no admin user.
	switch {
	case path == "/api/setup/status" && method == http.MethodGet:
		t.setupStatus(w, r)
		return
	case path == "/api/setup/bootstrap" && method == http.MethodPost:
		t.setupBootstrap(w, r)
		return
	case path == "/api/cluster/enroll" && method == http.MethodPost:
		// A joining node has no user identity yet; its join token (in the body) is
		// the credential, verified in enrollNode. Same exemption as bootstrap.
		t.enrollNode(w, r)
		return
	}

	// When auth is enabled every request must present a valid credential; when it
	// is disabled the console proceeds anonymously (the Policy is then a no-op too).
	principal, authed := t.authenticate(r)
	if t.Auth != nil && t.Auth.Enabled && !authed {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ctx := valuerpc.ContextWithPrincipal(r.Context(), principal)

	// admin reports whether the caller holds cluster-admin (gates the admin
	// console and admin-only operations); canRead reports whether the caller can
	// see the read-only dashboard (roles/cdb.viewer and up all hold records.get).
	admin := t.Policy.Authorize(ctx, iam.PermClusterAdmin, "", "") == nil
	canRead := t.Policy.Authorize(ctx, iam.PermRecordsGet, "", "") == nil

	authorize := func(perm string) bool {
		if err := t.Policy.Authorize(ctx, perm, "", ""); err != nil {
			writeErr(w, http.StatusForbidden, "permission denied")
			return false
		}
		return true
	}

	switch {
	case path == "/api/me" && method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"principal": principal, "isAdmin": admin, "canRead": canRead})

	case path == "/api/cluster" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) {
			t.cluster(w)
		}
	case path == "/api/ledger/status" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) {
			t.ledgerStatus(w)
		}
	case path == "/api/stats" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) {
			t.stats(w)
		}
	case path == "/api/regions" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) {
			t.regions(w)
		}
	case path == "/api/node/metrics" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) { // peers call this during fan-out; dashboard Nodes tab
			t.nodeMetricsEndpoint(w)
		}
	case path == "/api/cluster/nodes" && method == http.MethodGet:
		if authorize(iam.PermRecordsGet) { // dashboard Nodes tab (viewer+)
			t.clusterNodes(w, r)
		}

	// Admin-only: add / remove cluster members.
	case path == "/api/cluster/nodes" && method == http.MethodPost:
		if authorize(iam.PermClusterAdmin) {
			t.addNode(w, r)
		}
	case strings.HasPrefix(path, "/api/cluster/nodes/") && method == http.MethodDelete:
		if authorize(iam.PermClusterAdmin) {
			t.removeNode(w, r, strings.TrimPrefix(path, "/api/cluster/nodes/"))
		}
	case path == "/api/cluster/join-token" && method == http.MethodPost:
		if authorize(iam.PermClusterAdmin) {
			t.mintJoinTokenHandler(w, r, principal)
		}
	case path == "/api/ledger/verify" && method == http.MethodPost:
		if authorize(iam.PermProofsRead) {
			t.startVerify(w, r)
		}
	case path == "/api/ledger/verify" && method == http.MethodGet:
		if authorize(iam.PermProofsRead) {
			writeJSON(w, http.StatusOK, map[string]any{"jobs": t.Jobs.List()})
		}
	case strings.HasPrefix(path, "/api/ledger/verify/") && method == http.MethodGet:
		if authorize(iam.PermProofsRead) {
			id := strings.TrimPrefix(path, "/api/ledger/verify/")
			if job, ok := t.Jobs.Get(id); ok {
				writeJSON(w, http.StatusOK, job)
			} else {
				writeErr(w, http.StatusNotFound, "no such job")
			}
		}

	// Admin-only: database export/import and ledger CA generation.
	case path == "/api/database/export" && method == http.MethodGet:
		if authorize(iam.PermBackupsCreate) {
			t.exportDatabase(w, r)
		}
	case path == "/api/database/import" && method == http.MethodPost:
		if authorize(iam.PermBackupsRestore) {
			t.importDatabase(w, r)
		}
	case path == "/api/setup/ledger-ca" && method == http.MethodPost:
		if authorize(iam.PermClusterAdmin) {
			t.generateLedgerCA(w)
		}

	// Admin: IAM management — users, service accounts (application tokens),
	// roles, and role bindings. Reads need cdb.iam.get, writes cdb.iam.set.
	case path == "/api/iam/users" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListUsers(w)
		}
	case path == "/api/iam/users" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamCreateUser(w, r)
		}
	// Personal access tokens (PATs) per user — must precede the user-delete case,
	// which would otherwise match these longer paths.
	case strings.HasPrefix(path, "/api/iam/users/") && strings.HasSuffix(path, "/tokens") && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListUserTokens(w, strings.TrimSuffix(strings.TrimPrefix(path, "/api/iam/users/"), "/tokens"))
		}
	case strings.HasPrefix(path, "/api/iam/users/") && strings.HasSuffix(path, "/tokens") && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamCreateUserToken(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/api/iam/users/"), "/tokens"))
		}
	case strings.HasPrefix(path, "/api/iam/users/") && strings.Contains(path, "/tokens/") && method == http.MethodDelete:
		if authorize(iam.PermIamSet) {
			parts := strings.SplitN(strings.TrimPrefix(path, "/api/iam/users/"), "/tokens/", 2)
			t.iamRevokeUserToken(w, parts[0], parts[1])
		}
	case strings.HasPrefix(path, "/api/iam/users/") && method == http.MethodDelete:
		if authorize(iam.PermIamSet) {
			t.iamDeleteUser(w, strings.TrimPrefix(path, "/api/iam/users/"))
		}
	case path == "/api/iam/service-accounts" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListServiceAccounts(w)
		}
	case path == "/api/iam/service-accounts" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamCreateServiceAccount(w, r)
		}
	case strings.HasPrefix(path, "/api/iam/service-accounts/") && method == http.MethodDelete:
		if authorize(iam.PermIamSet) {
			t.iamDeleteServiceAccount(w, strings.TrimPrefix(path, "/api/iam/service-accounts/"))
		}
	case path == "/api/iam/roles" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListRoles(w)
		}
	case path == "/api/iam/bindings" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListBindings(w)
		}
	case path == "/api/iam/bindings" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamChangeBinding(w, r, true)
		}
	case path == "/api/iam/bindings/revoke" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamChangeBinding(w, r, false)
		}
	case path == "/api/iam/certs" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListCerts(w, r)
		}
	case path == "/api/iam/certs/register" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamRegisterCert(w, r)
		}
	case path == "/api/iam/certs/issue" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamIssueCert(w, r)
		}
	case path == "/api/iam/certs" && method == http.MethodDelete:
		if authorize(iam.PermIamSet) {
			t.iamRevokeCert(w, r.URL.Query().Get("identity"))
		}
	case path == "/api/iam/groups" && method == http.MethodGet:
		if authorize(iam.PermIamGet) {
			t.iamListGroups(w)
		}
	case path == "/api/iam/groups" && method == http.MethodPost:
		if authorize(iam.PermIamSet) {
			t.iamSetGroup(w, r)
		}
	case strings.HasPrefix(path, "/api/iam/groups/") && method == http.MethodDelete:
		if authorize(iam.PermIamSet) {
			t.iamDeleteGroup(w, strings.TrimPrefix(path, "/api/iam/groups/"))
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
