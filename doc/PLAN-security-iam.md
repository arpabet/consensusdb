# Plan: authentication, IAM, record signing, and the verifiable ledger

Research + design for productionizing consensusdb as a **shared, multi-tenant,
3-node cluster** with real security: authn (password → mTLS), GCP-style IAM,
owner-signed records, and cryptographic proof that the consensus actually
happened — "people should see consensus algos here, not just a name."

---

## 1. Market research

### Competitor matrix

| System | Consensus | AuthN | AuthZ | Crypto verifiability | Status / lesson |
|---|---|---|---|---|---|
| **Amazon QLDB** | internal | IAM (AWS) | IAM | SHA-256 journal hash chain + Merkle digests, proof API | **Shut down 2025-07-31.** AWS pointed users at Aurora + audit logging — which *loses* cryptographic verifiability. Killer lesson: QLDB proofs required the AWS API, so when the endpoint died, the proofs died. **Proofs must verify offline, without the vendor.** |
| **immudb** (Codenotary) | single-node primary / replicas | user/pass, sessions | per-DB users, SQL grants | Append-only ledger, Merkle tree per tx, **client-side verified reads/writes** (`VerifiedSet/Get`), auditors | The OSS leader in this space; active (2026: immutable audit logging, deeper Postgres compat). The capability bar to match. |
| **SQL Server / Azure SQL ledger** | n/a (RDBMS) | SQL/AAD | SQL RBAC | Per-row SHA-256 fingerprints chained per table + **database digests anchored to external WORM storage** (Blob immutability / Confidential Ledger); protects against DBAs | The big trend signal: **ledger as a feature of a general-purpose DB**, not a separate product. Updatable + append-only ledger table variants. |
| **etcd** | raft | password **and** mTLS (`--client-cert-auth`: cert **CN becomes the user**; password wins if both) | users → roles → **key-range/prefix permissions**, root role | none | The direct architectural peer (raft KV). Its RBAC (range-scoped perms, CN mapping) is the proven minimal model; ours adds tenancy + groups + custom roles on top. |
| **Dolt / TerminusDB / ScalarDL** | varies | varies | varies | Git-style versioning / ledger middleware | QLDB-refugee alternatives; niche. |
| **CockroachDB / TiKV / FoundationDB** | raft/paxos | certs, SQL users | SQL RBAC / none | none | Consensus without verifiability — the gap consensusdb can fill. |

### Regulatory pull (the accounting use case)

- **GoBD** (DE): bookkeeping records must be **tamper-proof through technical
  means, not policy** — "even a superuser can't modify a record without the
  change being detectable by an outside party"; tamper-evident access trails;
  10-year retention (HGB §257, AO §147).
- **eIDAS** (EU): signatures must be verifiable and linked to the signer; audit
  trails prove signer identity + document integrity.
- E-invoicing mandates (EU ViDA and national schemes) keep expanding — every
  invoice becomes a signed, archived, verifiable record.

An accounting app on consensusdb gets this as **platform capability** instead of
building it per-app.

### Trends distilled

1. **Ledger-as-a-feature won; ledger-as-a-product died** (SQL Server ledger vs
   QLDB). consensusdb should expose verifiability as a capability of a normal KV.
2. **Client-side / offline verifiability** is the differentiator (immudb) and
   the anti-vendor-lock lesson (QLDB). Proof material must be exportable and
   verifiable with open code + public keys only.
3. **External anchoring**: periodic digests pushed to WORM/object-lock storage
   so even cluster admins can't rewrite history undetected (SQL Server model,
   GoBD requirement).
4. **mTLS identity is table stakes** for infra services (etcd CN mapping,
   SPIFFE-style workload identity in k8s); password/token remains for humans
   and bootstrap.
5. **Consensus + verifiability is an open niche**: raft stores (etcd, CRDB)
   have no proofs; proof stores (immudb) have no real multi-node consensus.
   consensusdb sits exactly at the intersection — hence the name promise.

---

## 2. What we already have (inventory — this is most of the work)

| Primitive | Where | Role in this plan |
|---|---|---|
| Credential → principal handshake | value-rpc `SetCredential` / `SetAuthenticator`, principal-bound resumption (anti-replay reverse hash chain) | AuthN entry point |
| Principal in every handler ctx | `valuerpc.PrincipalFromContext` (injected in `serving_client.go`) | AuthZ enforcement point in `kv.*` handlers |
| mTLS end-to-end | `vrpc-server` `tls://`/`quic://` + `client-auth` + `tls-ca`; `valuerpc.PeerCertificates`; cdb `WithTLSConfig` | Cert-based authn (CN/SAN → principal, etcd-style) |
| Canonical, deterministic encoding | `value.Marshal/Pack` (Phase B: raft commands already canonical bytes) | Hash-chain + signing input |
| Canonical signing projection | `value.SignBytes(obj, "sign")` (depecher-proven, domain-separated) | Record signatures |
| Deterministic versions | version = raft log index, stamped in FSM | Proof addressing (`key@version`) |
| Multi-tenant key model | major=tenant / region / minor | IAM resource scopes |
| Watch fed from apply path | WatchHub | IAM cache invalidation; audit streaming |
| Encryption at rest + client-side sealing | badger master key; `store/middleware/crypto` | Compliance stack |
| Backup/restore engine | badger `Backup(w, since)` (full + incremental) + raft snapshots | Periodic backups |
| Metrics endpoint | `servion.MetricsHandler()` already on :8441 `/metrics` | Prometheus scrape target |
| Serf discovery + raft membership | `raftmod/serf_server.go`, control-plane Bootstrap/Join | 3-node k8s bootstrap |
| CLI framework + embedded SPA pattern | cligo; staphi go-bindata dashboard | Admin CLI + console |

Missing: the IAM model itself, signature fields + verify-on-write, the FSM hash
chain + checkpoints, admin API/console, backup automation, 3-node infra wiring.

---

## 3. Proposed architecture

### 3.1 AuthN ladder (all methods resolve to one `principal` string)

Principal convention (GCP-style): `user:alice`, `serviceAccount:staphi`,
`group:accounting` (groups are membership, never a login).

1. **Username + password over TLS** (basic): credential = value.Map
   `{method:"password", user, pass}`; server checks argon2id hash stored in the
   system tenant. For humans and bootstrap.
2. **Bearer token / API key**: credential = `{method:"token", token}`; tokens
   minted for service accounts by the admin API (random secret, hashed at rest,
   expiry + revocation). For apps that can't do mTLS.
3. **mTLS with the common CA** (target state for services): `client-auth: true`
   already verifies the cert; the Authenticator maps SAN URI (or CN) →
   registered `serviceAccount:` principal — etcd's proven model (CN-as-user).
   Password/token, if also presented, takes precedence (etcd semantics).

All three plug into the existing `SetAuthenticator`; reconnects re-authenticate
automatically (value-rpc re-sends the credential; session resumption is already
principal-bound and replay-hardened).

### 3.2 IAM (GCP-style, stored in the DB itself)

- **Permissions** (fixed verb list the server enforces):
  `cdb.records.{get,put,delete,increment,batch,enumerate,watch}`,
  `cdb.proofs.{read}`, `cdb.tenants.{create,delete,list}`,
  `cdb.iam.{get,set}`, `cdb.backups.{create,restore}`, `cdb.cluster.{admin}`.
- **Roles** = named permission lists. Predefined: `roles/cdb.viewer`,
  `roles/cdb.editor`, `roles/cdb.auditor` (read + proofs, nothing else),
  `roles/cdb.tenantAdmin`, `roles/cdb.admin`. **Custom roles** = user-created
  text-form role with an explicit permission list (exactly the GCP shape).
- **Bindings**: `{role, members[]}` attached at a scope: **instance → tenant
  (major key) → region**, inherited downward. A member is `user:`, `group:`,
  or `serviceAccount:`; groups expand (one level) at evaluation.
- **Storage**: reserved system tenant (`__system` major key): `iam/users/*`,
  `iam/serviceAccounts/*`, `iam/groups/*`, `iam/roles/*`, `iam/policy/*`,
  value-encoded records. IAM writes are normal raft commands ⇒ replicated,
  versioned, **hash-chained and auditable like everything else** (IAM changes
  land in the same verifiable ledger). Each node keeps a compiled in-memory
  policy cache invalidated by a `__system` watch.
- **Enforcement**: one `authorize(ctx, permission, tenant, region)` check at the
  top of every `kv.*`/admin handler using `PrincipalFromContext`. Data-plane
  requests already carry tenant+region in the key. Root/bootstrap: first-run
  `consensusdb iam bootstrap` creates the initial admin (like etcd's root).
- **Tenant isolation hard floor**: even `roles/cdb.editor` on tenant A can never
  touch tenant B — bindings are scope-checked before role evaluation.

### 3.3 Record signing (non-repudiation for accounting)

- Each principal may register one or more **Ed25519 public keys** in IAM
  (`iam/keys/<principal>/<keyId>`, with status active/revoked + validity window).
- **Wire**: `RecordRequest` gains `SignerKeyId string` + `OwnerSig []byte`.
  The signature covers the canonical projection via `value.SignBytes` over a
  domain-separated struct (the depecher `*SigInput` pattern):
  `{dom:"cdb/record/v1", tenant, region, minorKey, valueHash(SHA-256), ttl/expiry}`.
  Signing the value **hash** keeps signatures small and works for streams.
- **Verify-on-write at the leader, before the command enters the raft log**
  (deterministic — replicas never re-verify at apply). Policy per region:
  `unsigned | optional | required` (append-only accounting regions run
  `required`). Invalid/revoked-key signatures are rejected with a typed error.
- **Stored** in the record head (alongside version/expiry); returned on reads
  and in watch events. `kv.verify` needs nothing server-side: an **offline
  auditor** re-derives SignBytes from the record + fetches the signer's public
  key (exportable) — verification works with open code only (the QLDB lesson).
- **With client-side encryption** (crypto middleware): default is
  sign-what-you-store (signature over the sealed bytes' hash — proves the exact
  stored blob came from the key holder). Apps that need auditors to verify
  *plaintext* without decryption keys sign at app level inside the value.
  Documented pattern, not a platform fork.
- Capability: cdb provider advertises `Signed`; staphi/accounting apps can
  require it.

### 3.4 The verifiable ledger — making the consensus visible

This is the showpiece, and Phase B made it cheap: **raft command bytes are
already canonical**.

1. **FSM hash chain**: at apply, each node computes
   `chain[i] = SHA-256(chain[i-1] ‖ index ‖ term ‖ commandBytes)` and persists
   `(index, chain[i])` in the system region. Purely deterministic ⇒ every
   replica derives the *identical* chain. Divergence = corruption/tamper, and
   it's *detectable*, not silent.
2. **Signed checkpoints**: every N entries / T seconds the leader emits
   `Checkpoint{index, term, chainDigest, wallClock}` signed with its **node
   key** (Ed25519, from the cluster CA'd identity); followers that agree
   co-sign. A checkpoint with **quorum signatures is a consensus certificate**
   — cryptographic, third-party-checkable evidence that a majority of nodes
   agreed on exactly this history. This is the literal answer to "people should
   see consensus algos here."
3. **APIs**: `kv.digest` (latest quorum checkpoint), `kv.audit` (stream
   entries + chain links since index X, so an external auditor replays and
   re-derives the digest), checkpoint history browsing.
4. **External anchoring** (SQL Server ledger model, GoBD requirement): a tiny
   cron/console job pushes each checkpoint to WORM/object-lock storage (and/or
   prints it to logs, email, even a public place). Cluster admins can rewrite
   the disk but not the anchored digests.
5. **v2 — inclusion proofs** (immudb parity): swap the linear chain for a
   Merkle Mountain Range over the same leaves so `kv.proof(key@version)` returns
   an O(log n) inclusion proof under any checkpoint, plus consistency proofs
   between checkpoints (Certificate-Transparency-style). The linear chain (v1)
   already gives tamper-evidence + full-audit; MMR adds cheap point proofs.

### 3.5 Productionization (3-node shared cluster)

- **Topology**: `num_replicas = 3`; raft+serf enabled via env
  (`RAFT_BIND_ADDRESS`, `SERF_BIND_ADDRESS`, serf join =
  `consensusdb-0.consensusdb-headless…:port`); pod **anti-affinity** across
  nodes; **PodDisruptionBudget** `maxUnavailable: 1`; headless svc already has
  `publishNotReadyAddresses`. Ordinal-0 bootstraps once; others Join (control
  plane already has Bootstrap/Join).
- **Metrics**: `/metrics` exists; wire **hashicorp/raft** stats (go-metrics →
  Prometheus sink: leader, term, commit index, apply latency, fsm pending),
  **badger** (lsm/vlog size, compactions), **vrpc** (per-fn latency/error),
  IAM (authn failures, denials), ledger (checkpoint lag). Prometheus scrape
  annotations / PodMonitor in terraform. Alerts: no-leader, follower lag,
  checkpoint stall, disk %, backup age.
- **Logs**: zap already structured; make prod mode env-driven (`main.go`
  currently hardcodes `ZapLogFactory(true)` = dev) → JSON to stdout for the
  cluster collector. Add an **access/audit log** stream (authn result,
  principal, permission decision) — GoBD wants access trails too.
- **Backups**: admin API `admin.backup` streams badger `Backup(w, since)`;
  `consensusdb backup` CLI writes it to a file/S3-compatible endpoint. K8s
  **CronJob**: nightly full + hourly incremental (`since` = last version),
  uploaded to object storage **with object-lock/WORM** (compliance story
  complete: signed records + anchored checkpoints + immutable backups).
  `consensusdb restore` into a fresh node → re-bootstrap; runbook in README.
  Raft snapshots stay for log compaction (they're not backups).
- **Certs/keys**: common CA via cert-manager (server certs, client service
  certs, node signing keys); master-key + node-key rotation procedure.

### 3.6 Admin console (phased, don't start with the UI)

1. **CLI** (cligo, talks vrpc admin API): `iam user|sa|group|role|binding …`,
   `tenant …`, `backup|restore`, `checkpoint list|anchor|verify`, `cluster status`.
2. **Admin vrpc API** (`admin.*` functions, `cdb.iam.*` / `cdb.cluster.admin`
   gated) — one API serving CLI and console identically.
3. **Web console**, embedded SPA (exact staphi pattern: go-bindata + servion on
   :8441): IAM CRUD (users/groups/SAs/roles/bindings, GCP-console-like),
   tenants/regions with signing policy, **cluster panel** (leader, term, peers,
   commit/applied index — raft made visible), **ledger panel** (checkpoints,
   quorum signatures, verify button, anchor status), backups (schedule, age,
   restore points), metrics summary.

---

## 4. Roadmap

| Phase | Deliverable | Size |
|---|---|---|
| **S1** ✅ DONE 2026-07-03 | 3-node infra: raft/serf env in statefulset (per-ordinal bootstrap via pod hostname), anti-affinity, PDB, `num_replicas=3`, raft+serf ports on container/headless svc, bootstrap/join runbook (README); prod zap via `COS=prod`; raft metrics (go-metrics→prometheus sink, armon+hashicorp globals) + badger expvar collector on `/metrics` (`pkg/run/metrics.go`); `consensusdb raft config\|join\|bootstrap` CLI registered (`cmd.RaftGroup` roots the parentless raftvrpc group) | S |
| **S2** ✅ DONE 2026-07-03 | AuthN: `pkg/iam` (system-tenant layout `__system`/IAM: `user/*` `sa/*` `cert/*`, value-encoded records, argon2id passwords, `<sa>.<secret>` tokens) + `server.AuthService` authenticator (password/token/mTLS-SAN-or-CN → principal; explicit credential wins, etcd semantics; installed on the data plane when `auth.enabled=true`) + `iam bootstrap\|user-add\|sa-add` CLI (etcd-style enablement flow, `iam.address`/`IAM_USER`… credentials) + cdb `WithCredential` / `PasswordCredential` / `TokenCredential` (re-presented on reconnects/redirects) + `auth_enabled` terraform toggle + README runbooks. Proven by `TestCdbAuthPasswordAndToken` + `TestCdbAuthMutualTLSIdentity` (race-clean) | M |
| **S3** ✅ DONE 2026-07-03 | IAM authorization: `iam.Snapshot` evaluator (permissions/roles/bindings, instance→tenant→region inheritance, group expansion, predefined+custom roles, tenant-isolation floor + `__system`→cdb.iam.* mapping) + `server.PolicyService` (compiles snapshot from system tenant, atomic swap, `__system` watch invalidation → cluster-wide reload no restart) + per-handler guards in `vrpc_data.go` (scope from request key) + `iam role-add\|group-set\|binding-add` CLI. Denials logged (authz audit). Proven by `TestCdbAuthorizationEnforced` (allow/deny by role, isolation floor, `__system` guard, live watch reload) + `pkg/iam` policy unit tests | M–L |
| **S4** ✅ DONE 2026-07-03 | Backups: `pkg/backup` (streaming argon2id+AES-256-GCM container w/ plain mode + truncation/tamper detection; file + s3:// sinks via minio-go = AWS/MinIO/GCS, object-lock WORM) + `server.AdminService` (admin.backup outgoing stream + admin.restore chat, cdb.backups.* gated, restore refused while replication active) + `consensusdb backup\|restore` CLI (client-side encryption + S3 creds) + CronJob→object-lock in terraform. Incremental via badger since. Proven by `TestBackupRestoreRoundTrip` (encrypted round-trip into fresh node + wrong-password reject) + `pkg/backup` crypto/objstore unit tests. Backup-age metric/alert = follow-up | S–M |
| **S5** | Signing: key registry, `SignerKeyId/OwnerSig` on the wire, verify-on-write w/ per-region policy, `Signed` capability in cdb provider, offline-verifier example | M |
| **S6** | Verifiable ledger v1: FSM hash chain, quorum-signed checkpoints, `kv.digest`/`kv.audit`, checkpoint anchoring job + verify CLI | M |
| **S7** | Admin console SPA on the admin API | M–L |
| **S8** | Ledger v2: MMR inclusion/consistency proofs (`kv.proof`) — immudb parity | L |

S1–S2 unblock the shared cluster; S3 makes multi-tenancy safe to open up;
S5+S6 are the accounting differentiator and can proceed in parallel with S4.

## 5. Decisions needed

1. **Principal naming**: plain (`user:alice`) vs email-form (`user:alice@corp`)?
2. **mTLS mapping source**: cert CN, or SAN URI (SPIFFE-style
   `spiffe://karagatan/sa/staphi`)? SAN URI recommended (CN is legacy).
3. **Signing scope default** for encrypted values: sign sealed bytes (default
   proposed) — confirm, given auditors' needs in the accounting app.
4. **Checkpoint cadence + anchor target** (every N=1024 entries or 60s;
   S3-object-lock bucket?).
5. **Console stack**: same Vue/React+go-bindata approach as staphi?

## 6. Sources

- QLDB shutdown + lesson: [InfoQ](https://www.infoq.com/news/2024/07/aws-kill-qldb/), [Certyo migration retro](https://www.certyos.com/en/blog/aws-qldb-migration-lesson), [AWS Aurora migration path](https://aws.amazon.com/blogs/database/replace-amazon-qldb-with-amazon-aurora-postgresql-for-audit-use-cases/), [DoltHub on alternatives](https://www.dolthub.com/blog/2024-08-12-qldb-deprecated-alternatives/)
- immudb model: [docs — how it works](https://docs.immudb.io/0.8.1/how-it-works.html), [research paper](https://codenotary.s3.amazonaws.com/Research-Paper-immudb-CodeNotary_v3.0.pdf), [2026 release](https://www.businesswire.com/news/home/20260505298955/en/Open-Source-Tamper-Proof-Database-Adds-Immutable-Audit-Logging-and-Expands-PostgreSQL-Compatibility)
- SQL Server ledger: [Microsoft Learn — overview](https://learn.microsoft.com/en-us/sql/relational-databases/security/ledger/ledger-overview?view=sql-server-ver17), [digest verification](https://learn.microsoft.com/en-us/sql/relational-databases/security/ledger/ledger-verify-database?view=sql-server-ver17), [QLDB→Azure SQL ledger](https://techcommunity.microsoft.com/blog/azuresqlblog/moving-from-amazon-quantum-ledger-database-qldb-to-ledger-in-azure-sql/4246237)
- etcd auth/RBAC: [RBAC guide](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/), [transport security / cert CN auth](https://etcd.io/docs/v3.6/op-guide/security/)
- Accounting compliance: [GoBD-compliant archiving](https://originstamp.com/en/blog/reader/gobd-compliant-archiving-e-invoice-requirements), [revisionssichere Archivierung](https://originstamp.com/en/blog/reader/revisionssichere-archivierung-e-invoices-guide), [audit-trail compliance guide](https://chaindoc.io/blog/audit-trail-compliance-guide)
