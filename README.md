# consensusdb

ConsensusDB Database

A strongly consistent, raft-replicated key-value store that serves the
`go.arpabet.com/store` interface over the network. It lets any store-based
application drop its embedded engine (badger, pebble, bbolt, …) and become
**stateless**: state moves into a small consensusdb cluster while the app keeps
the exact same `DataStore` API — versioned compare-and-set, TTL, ordered
enumeration, and watch — with no application code changes.

Writes go through a raft leader and are committed to a replicated log applied
identically on every node, so reads are linearizable and CAS is safe across
replicas. A single node bootstraps as its own leader and grows into a
multi-node cluster; there is no external dependency and no minimum node count.
See [doc/PLAN-store-provider.md](doc/PLAN-store-provider.md) for the roadmap
turning this into the `store/providers/cdb` client.

Payloads are sealed **client-side** by default (the client encrypts before the
wire), so a compromised node yields only ciphertext — strictly stronger than
encryption-at-rest.

# Description

* value-rpc data plane (TCP / TLS / mTLS / QUIC) — the sole API, consumed by the
  `store/providers/cdb` client (gRPC and the REST/JSON gateway were retired)
* Engine - badger (similar to rocksdb, but faster)
* Raft replication (hashicorp/raft via sprint raftmod) — leader-based, strongly
  consistent, snapshot/restore
* Serves the `go.arpabet.com/store` capability contract: TTL, versioned CAS,
  ordered enumeration, watch
* Multi-tenant key model (majorKey / region / minorKey), point-in-time by TimeUUID
* Client-side sealed encryption: AES-GCM / AES-CFB, per-tenant block keys
* Supports compression: Snappy, LZ4
* Pure golang implementation

# Current Status
* Active development. Working today: the raft-replicated engine with envelope
  versioning (version = raft log index), CAS / increment / atomic batch as raft
  ops, leader-computed TTL with a deterministic reclaimer, a cross-node watch hub,
  raft membership (bootstrap / join), the value-rpc control + data planes, and the
  `go.arpabet.com/store/providers/cdb` client (a `store.DataStore` speaking the
  value-rpc data plane, with leader redirect, ordered enumeration and encryption).

# Design
Data collocated by majorKey in data nodes, grouped by regionName to reference different types of data, accessible by minorKey and TimeUUID.
All data records are ordered by majorKey/regionName/minorKey/TimeUUID(timestamp, counter)
MajorKey points to the tenant, that how multi-tenant architecture is supported.

# Best practices
* Use userId in key.MajorKey, for example "accountNumber", "nickname", "incremental id" or other primary identifier in multi-tenant systems.
* Use table name in key.RegionName in upper case, for example "ACCOUNT", "PROFILE", "CHAT", "AUTH".
* Use other userId in key.MinorKey with whom we record interaction or type of the event, for example "accountNum", "login"
* Create TimeUUID based on event content and timestamp for multi-datacenter support (MDC)
* Store event with TimeUUID, store record without TimeUUID

# Encryption

Two independent, composable layers:

* **At rest (server-side).** Set `consensusdb.encryption-key` to a base64 (RawURL)
  AES-256 master key — e.g. one produced by the `seal` command — and badger
  encrypts the LSM tables and value log on disk. Empty means unencrypted. A store
  written with a key can only be reopened with the same key. This protects against
  someone obtaining the on-disk files.

  ```
  ./consensusdb seal                 # prints a fresh master key
  CONSENSUSDB_ENCRYPTION_KEY=<key> ./consensusdb run
  ```

* **In transit / end-to-end (client-side).** Wrap the `cdb` provider with store's
  crypto middleware (`cryptostore.New(cdb.New(...), key)`): values are AES-GCM
  sealed before they leave the client, so **the server only ever stores
  ciphertext**. This protects against a compromised server. Keys stay plaintext
  (they must remain comparable for the key layout); only values are sealed.

Use either, or both together for defense in depth.

# Enumeration and ordering

Keys are encoded with a 2-byte length prefix per field
(`[majorLen][major][regionLen][region][minorLen][minor][sortable-TimeUUID]`). The
length prefix delimits the variable-length binary fields unambiguously **without
escaping**, which lets a whole tenant (or tenant/region) be scanned as a clean
byte-prefix — the hierarchical multi-tenant access pattern this store is built for.
The trailing TimeUUID uses an order-preserving encoding, so a row's versions stay
ordered.

The trade-off: the length prefix does **not** preserve lexical byte-order *within*
a field. So the `store` provider, which wants flat lexical key ranges, asks for
**opt-in server-side ordering**: `kv.enumerate` takes an `ordered` flag, and when
set the server sorts the scanned region by the decoded minor key (lexical, reverse
on request) before streaming. That is what lets `store/providers/cdb` advertise the
`Ordered` capability.

# Secure transport — TLS, mutual TLS, QUIC

The value-rpc **data plane** (`vrpc-server`) can run over plain TCP, TLS, mutual
TLS, or QUIC. The transport is chosen by the **scheme** of `vrpc-server.bind-address`,
and TLS material is loaded from the filesystem:

| `vrpc-server.bind-address` | Transport | Requires |
|---|---|---|
| `tcp://0.0.0.0:8444`   | plain TCP        | — |
| `tls://0.0.0.0:8444`   | TLS over TCP     | `tls-cert`, `tls-key` |
| `quic://0.0.0.0:8444`  | QUIC (TLS/UDP)   | `tls-cert`, `tls-key` |
| `unix:///run/cdb.sock` | Unix socket      | — |

```yaml
vrpc-server.bind-address: tls://0.0.0.0:8444
vrpc-server.tls-cert:     /etc/cdb/server.crt   # PEM server certificate
vrpc-server.tls-key:      /etc/cdb/server.key   # PEM private key
# Mutual TLS: require and verify a client certificate against a CA bundle.
vrpc-server.client-auth:  true
vrpc-server.tls-ca:       /etc/cdb/ca.crt       # CA that signs client certs
```

With `client-auth: true` the server **requires and verifies** a client certificate
(mutual TLS); the verified identity is available to handlers via
`valuerpc.PeerCertificates`, so callers can be authorized by certificate. QUIC is
always encrypted and is the fastest option on private networks / kubernetes.

The **client** (`store/providers/cdb`) selects the matching transport by address
scheme and passes certificates with `cdb.WithTLSConfig`:

```go
mtls := &tls.Config{RootCAs: caPool, Certificates: []tls.Certificate{clientCert}, ServerName: "consensusdb-1"}
ds, _ := cdb.New("app", "tls://consensusdb-1:8444", "acme", "USERS", cdb.WithTLSConfig(mtls))
// or "quic://consensusdb-1:8444" for QUIC
```

Because the provider transparently redials the leader on a redirect, use
certificates whose SANs cover **all** node hostnames/IPs (or a shared `ServerName`).
`TestCdbMutualTLS` and `TestCdbQUIC` (in `pkg/replication`) exercise both ends end
to end. See the `store/providers/cdb` README for the full client matrix.

# Authentication

The data plane authenticates every connection when `auth.enabled=true`
(`AUTH_ENABLED=true`). Three methods, all resolving to one **principal** that
reaches every handler (authorization builds on it in a later phase):

| Method | Credential (value-rpc handshake) | Principal |
|---|---|---|
| Username + password | `{method:"password", user, pass}` — argon2id at rest | `user:<name>` |
| API token | `{method:"token", token}` — `<sa>.<secret>`, sha256 at rest | `serviceAccount:<name>` |
| mTLS client certificate | none — the verified cert's **SAN URI** (or CN) is looked up in the cert index | `serviceAccount:<name>` |

An explicit credential wins over the peer certificate (etcd semantics); the
credential is re-presented automatically on every reconnect and leader redirect.
Identities are records in the system tenant (`__system`/`IAM` — replicated and
versioned like any data): `user/<name>`, `sa/<name>`, `cert/<identity>`.

**Enablement (etcd-style, no chicken-and-egg):**

```bash
# 1. while auth is disabled, create the identities on a running node
consensusdb iam bootstrap admin --password '…'      # initial admin (or generated+printed)
consensusdb iam user-add alice                       # humans (password printed once)
consensusdb iam sa-add staphi                        # workloads (token printed once)
consensusdb iam sa-add webby --cert-idents urn:cdb:sa:webby   # mTLS identity

# 2. enable and restart
AUTH_ENABLED=true consensusdb run
```

The iam CLI dials `iam.address` (default `tcp://127.0.0.1:8444`); once auth is
enabled, give the CLI its own credential via `IAM_USER`/`IAM_PASSWORD` or
`IAM_TOKEN`. Clients authenticate with the cdb provider options:

```go
cdb.New("app", addr, tenant, region, cdb.WithCredential(cdb.PasswordCredential("alice", pass)))
cdb.New("app", addr, tenant, region, cdb.WithCredential(cdb.TokenCredential(token)))
// mTLS identity: a registered client certificate needs no credential at all
cdb.New("app", "tls://…:8444", tenant, region, cdb.WithTLSConfig(mtlsCfg))
```

On Kubernetes: deploy with `auth_enabled=false`, `kubectl exec consensusdb-0 --
/app/consensusdb iam bootstrap admin`, then set `auth_enabled=true` in
`terraform.tfvars` and re-apply. `TestCdbAuthPasswordAndToken` and
`TestCdbAuthMutualTLSIdentity` (in `pkg/replication`) exercise the full ladder.

# Authorization (IAM)

With `auth.enabled=true`, every data-plane operation is authorized against a
GCP-style policy: **permissions → roles → bindings**, evaluated for the
connection's principal at the addressed **(tenant, region)** scope.

- **Permissions** the server enforces: `cdb.records.{get,put,delete,increment,
  batch,enumerate,watch}`, and `cdb.iam.{get,set}` for the system tenant.
- **Roles** are permission lists. Predefined: `roles/cdb.viewer`,
  `roles/cdb.editor`, `roles/cdb.auditor`, `roles/cdb.tenantAdmin`,
  `roles/cdb.admin`. Custom roles are the GCP "text form" — a named permission
  list you create.
- **Bindings** `{role, members[]}` attach at **instance → tenant → region**,
  inherited downward; a grant is never broader than its binding's scope. Members
  are `user:…`, `serviceAccount:…`, or `group:…` (groups expand one level).
- **Tenant-isolation floor**: a binding on tenant A never authorizes tenant B —
  scope is the request key's own `major`/`region`, so a caller cannot address one
  tenant while authorized for another. The **system tenant** (`__system`, holding
  IAM itself) requires `cdb.iam.*` instead of `cdb.records.*`.

Policy lives in the system tenant (like identities) and is **replicated,
versioned, and auditable** the same way. Each node compiles it into an immutable
in-memory snapshot and reloads on any `__system` change (fed by the raft apply
path) — grants take effect cluster-wide **without a restart**. `bootstrap` admins
have full access.

```bash
# a read-only auditor role, granted to a group at a tenant
consensusdb iam group-set analysts --members user:carol,user:dave
consensusdb iam role-add roles/reports.viewer --permissions cdb.records.get,cdb.records.enumerate
consensusdb iam binding-add roles/reports.viewer --members group:analysts --tenant acme

# a service account that may only write one region
consensusdb iam binding-add roles/cdb.editor --members serviceAccount:staphi --tenant acme --region APP
```

`TestCdbAuthorizationEnforced` (in `pkg/replication`) drives the whole path:
allow/deny by role, the tenant-isolation floor, the `__system` guard, and the
live watch-driven policy reload.

# Verifiable ledger — seeing the consensus

consensusdb is not just raft-replicated; the agreed history is **cryptographically
verifiable**. Every committed raft entry is folded into a deterministic hash chain
in the FSM apply path:

	chain[i] = SHA-256( chain[i-1] ‖ index ‖ term ‖ commandBytes )

Because it is a pure function of the committed log, **every replica derives the
identical chain** — a divergent head is proof of corruption or tampering, and it
is *detectable*, not silent. The head is persisted (in a reserved `__ledger`
tenant) so it survives snapshots and restarts.

A quorum of nodes co-signs a **Checkpoint** of the chain head, and the aggregate
of those signatures is a **QuorumCertificate**: compact, third-party-checkable
evidence that a majority of the certified cluster agreed on exactly this history.

**Signatures use BLS12-381 (keys in G2), so each is 48 bytes and N node
signatures aggregate into a single 48-byte signature** — the smallest practical
footprint for a multi-signer certificate (versus N × 64 bytes for Ed25519). A
quorum certificate is the checkpoint + the signer id list + one 48-byte aggregate,
independent of cluster size. Node keys are certified by a common **Ed25519 CA**;
verification needs only the CA public key, the node certs, and the checkpoint —
**it works entirely offline, with no running service** (the anti-vendor-lock
lesson from QLDB).

CLI (the CA private key stays offline):

```bash
consensusdb ledger ca-init  ca.key ca.pub                 # once: the ledger CA
consensusdb ledger keygen   node0.key node0.pub           # per node: BLS key pair
consensusdb ledger issue    ca.key node-0 node0.key node0.cert   # CA vouches (w/ PoP)
# point each node at its identity: ledger.node-key / ledger.node-cert
# collect signed digests (ledger.digest) from a quorum, aggregate, then:
consensusdb ledger verify   ca.pub quorum.bin node0.cert node1.cert --threshold 2
```

`ledger.digest` returns a node's current signed checkpoint (authorized by
`cdb.proofs.read`; `roles/cdb.auditor` grants read + proofs and nothing else).
Anchoring each checkpoint to WORM object storage (via the backup infra) makes even
a cluster admin unable to rewrite the anchored history — a follow-up. The full
path is proven by `TestLedgerQuorumOverConvergedChain` (3 replicas converge → a
2-of-3 quorum certificate verifies offline, tamper fails) and the `pkg/ledger`
unit tests.

## Verifying a backup

Because the chain head is stored in every dump, a backup can be tied back to a
quorum certificate: `consensusdb ledger verify-backup` loads the dump into a
throwaway store, reads the persisted head, and confirms a quorum certificate
attests **exactly that head** — proving, entirely offline, that the backup is the
state a majority of the cluster certified at a height:

```bash
consensusdb ledger verify-backup s3://backups/cdb/full.dump ca.pub quorum.bin \
  node0.cert node1.cert --password "$PW" --threshold 2
# → VERIFIED ✓  height=… digest=… signers=2
```

Verification also runs as a **background job over REST** (for the web console): a
node's `POST /api/ledger/verify` starts a job and `GET /api/ledger/verify/{id}`
reports `{state, progress, result}` — a progress bar over a large dump. Both are
authorized by `cdb.backups`/`cdb.proofs.read`. `TestVerifyBackupAgainstQuorum`
proves the offline round trip (backup → load → match quorum, with mismatch
rejection); `pkg/console` tests the job/REST loop.

## Web admin console

The node serves a **Vue + Vite** admin console at **`/console`** (source in
`webapp/`, built into `webapp/dist`, baked into the image). It calls the admin
REST API under `/api` and shows the cluster/raft status, the live ledger head, and
a **backup-verification form with a progress bar** driven by the background job
above. Authenticate with an IAM token that has `cdb.proofs.read` (e.g.
`roles/cdb.auditor`). See `webapp/README.md` for dev/build.

# Backup & restore

`consensusdb backup|restore` streams a whole-store dump over the admin control
surface to/from a **local file** or **S3-compatible object storage** — AWS S3,
MinIO (open source, on-prem), or Google Cloud Storage's S3 endpoint, selected by
the endpoint alone. Dumps are **plain** or **password-protected** (argon2id →
AES-256-GCM, chunked so GB-scale streams need no buffering, with truncation and
tamper detection). **Encryption and the object-storage keys live on the client**,
so a node never holds a backup password or bucket credential.

```bash
# encrypted backup to S3-compatible storage (MinIO/AWS/GCS)
BACKUP_S3_ENDPOINT=minio.internal:9000 BACKUP_S3_ACCESS_KEY=… BACKUP_S3_SECRET_KEY=… \
  consensusdb backup s3://backups/cdb/full.dump --password "$PW"

# incremental (only entries after a previous backup's reported version)
consensusdb backup s3://backups/cdb/inc.dump --since 12345 --password "$PW"

# plain local dump
consensusdb backup /var/backups/cdb.dump

# restore INTO A FRESH NODE, then bootstrap the cluster
consensusdb restore s3://backups/cdb/full.dump --password "$PW"
```

Backup is read-only and safe on any node; it is authorized by `cdb.backups.create`
(`roles/cdb.admin` includes it). **Restore bypasses raft**, so it is refused while
replication is active — restore into a fresh node and then bootstrap (see the
cluster runbook). It is authorized by `cdb.backups.restore`.

**Scheduled off-site backups (WORM):** setting `backup_schedule` in
`terraform.tfvars` deploys a **CronJob** that runs `consensusdb backup` to
`backup_dest_prefix` (an `s3://…` URL) and writes each object with **object-lock
retention** (`backup_retain_days`) — a cluster admin cannot alter or delete a
backup until it expires (the compliance-grade off-site copy; the bucket must have
object-lock enabled). The dump password and S3 keys are Kubernetes secrets.
`TestBackupRestoreRoundTrip` (in `pkg/replication`) proves the full encrypted
round trip and the wrong-password rejection; the crypto and object-store layers
are unit-tested in `pkg/backup`.

# Deploy to Kubernetes (Karagatan)

The **`Deploy to Karagatan`** GitHub Action (`.github/workflows/deploy.yaml`) builds
the private image, pushes it to the Karagatan registry, and applies the stateful
Terraform config in `infra/` to the cluster. Trigger it by pushing a `v*` tag, or
manually (with a `deploy_only` option that skips the build and just re-applies).

Because consensusdb is **stateful**, `infra/` deploys a **StatefulSet** (not a
Deployment) with a per-replica `PersistentVolumeClaim` for the badger data
directory (`consensusdb.data-dir` → `/data`), a **headless service** for stable pod
identities, and a **ClusterIP service** for clients. The value-rpc data plane is
enabled (`VRPC_SERVER_BIND_ADDRESS=tcp://0.0.0.0:8444`) so the `store/providers/cdb`
client can connect. It is an internal service — no public gateway route.

It is a **shared, multi-tenant instance**: multiple services target the same
cluster and are isolated inside consensusdb by tenant (the cdb client's `tenant`
arg → the `major` key). It therefore runs in its own dedicated `consensusdb`
namespace. A stateless app (e.g. staphi) connects in-cluster via the service DNS:

```
STAPHI_CDB_ADDRESS=tcp://consensusdb.consensusdb.svc.cluster.local:8444
```

Required repository secrets: `REGISTRY_HOSTNAME` / `REGISTRY_USERNAME` /
`REGISTRY_PASSWORD` (private registry), `KUBE_CONFIG` (base64 kubeconfig), and
optionally `CONSENSUSDB_ENCRYPTION_KEY` (base64 AES-256 at-rest key, mounted as a
secret). Deployment values (namespace, sizes, ports) live in `infra/terraform.tfvars`;
Terraform state is stored in a Kubernetes secret in the namespace (`state.tf`), which
must already exist.

## 3-node cluster formation (runbook)

The StatefulSet deploys **3 raft voters** (`num_replicas = 3`): replication is
enabled via `RAFT_BIND_ADDRESS`/`SERF_BIND_ADDRESS`, ordinal **0 bootstraps** a
single-voter cluster on first start (`RAFT_BOOTSTRAP=true`, derived from the pod
ordinal), and ordinals 1–2 start in **join mode**. Pod anti-affinity spreads the
voters across nodes and a PodDisruptionBudget caps voluntary disruptions at one
voter, so maintenance never costs quorum.

Forming the cluster is a one-time step — add each joiner as a voter, addressed by
its **stable headless DNS name**:

```bash
# each joiner logs its node id on start:
kubectl -n consensusdb logs consensusdb-1 | grep RaftJoinMode   # → id=<node_id>

# the leader (pod 0 right after bootstrap) adds the voters:
kubectl -n consensusdb exec consensusdb-0 -- /app/consensusdb raft join \
  <node_id_1> consensusdb-1.consensusdb-headless.consensusdb.svc.cluster.local:8300
kubectl -n consensusdb exec consensusdb-0 -- /app/consensusdb raft join \
  <node_id_2> consensusdb-2.consensusdb-headless.consensusdb.svc.cluster.local:8300

# verify: three voters
kubectl -n consensusdb exec consensusdb-0 -- /app/consensusdb raft config
```

Membership is persisted in the raft log — restarts rejoin automatically. Two
operational notes:

- **Address changes**: joiners are recorded under DNS names (stable across pod
  restarts). The seed records its own advertised **pod IP** at bootstrap; if pod
  0 is rescheduled and peers can't reach its old IP, re-run `raft join <node_id_0>
  consensusdb-0.…:8300` from the current leader — `join` with an existing id
  updates that server's address.
- **Scaling up**: raise `num_replicas`, then `raft join` the new ordinal the same
  way. Scale-downs must `RemoveServer` before deleting the pod (leader-side; CLI
  follow-up).

Metrics for the cluster (raft leader contact, commit/apply latency, badger LSM
counters) are on `:8441/metrics` for Prometheus; logs are structured JSON when
`COS=prod` (set by the deployment).

# Quick start

Build, Run, Write Client

### Prerequisites

Install tools:
```
go install github.com/google/go-licenses@latest
```

The data model is plain Go structs encoded with the `go.arpabet.com/value`
framework (`pkg/pb/cdb.go`) — there is no protobuf/codegen step.

### Build
```
go build
```

Build for Linux Amd64
```
env GOOS=linux GOARCH=amd64 go build
```

### Open

```
open http://localhost:8441/
```


### Run
```
./consensusdb run
./consensusdb run -c consensus.yaml

```

### Check

```
lsof -n -i:$PORT | grep LISTEN
```

You have to see consensusdb listening on 8441 (http: health, metrics, welcome).
The value-rpc data plane binds 8444 when `vrpc-server.bind-address` is set.

### Go Client Example

The client is `go.arpabet.com/store/providers/cdb` — a `store.DataStore` over the
value-rpc data plane (the old in-tree gRPC SDK was removed). Wrap it with the
crypto middleware for client-side encryption.

```go
import (
    "go.arpabet.com/store"
    cdb "go.arpabet.com/store/providers/cdb"
)

// Dial the data plane; major=tenant, region=logical table (see providers/cdb).
ds, err := cdb.New("app", "tcp://localhost:8444", "alex", "ACCOUNT")
defer ds.Destroy()

// putIfAbsent (version 0) with a one-day TTL.
ok, err := ds.CompareAndSetRaw(ctx, []byte("balance"), []byte("1245.90"), 86400, 0)

// get / remove.
val, err := ds.GetRaw(ctx, []byte("balance"), nil, nil, false)
err = ds.RemoveRaw(ctx, []byte("balance"))

// ordered scan of the tenant/region.
err = ds.EnumerateRaw(ctx, nil, nil, 100, false, false, func(e *store.RawEntry) bool {
    return true
})
```

To address many tenants and regions over **one** connection — the native cdb
access model (MajorKey + RegionName per request) — use the multi-store:

```go
multi, _ := cdb.NewMulti("app", "tcp://localhost:8444")
users := multi.Region("alice", "USERS")     // tenant alice, table USERS
prof  := multi.Region(profileId, "PROFILE") // per-user collocation; views are O(1)
```

Regions are GemFire/GemStone-style logical tables; the major key is the
collocation unit (a tenant or profile id — one owner's data lives together,
scannable and movable as a whole). See the `store/providers/cdb` README for
TLS/mTLS/QUIC (`WithTLSConfig`), `MultiDataStore`, and the full capability matrix.

```

### Configuration

Properties (overridable via a `-c` config file or environment; env keys uppercase
the property and turn `.`/`-` into `_`, e.g. `VRPC_SERVER_BIND_ADDRESS`):

```
http-server.bind-address:   0.0.0.0:8441
vrpc-server.bind-address:   tcp://0.0.0.0:8444   # data plane; empty disables it
consensusdb.data-dir:       /tmp/consensusdb
consensusdb.encryption-key:                      # base64 AES-256, empty = off
```

### Influencers

* [MDCC](http://mdcc.cs.berkeley.edu/)
* [Megastore](https://storage.googleapis.com/pub-tools-public-publication-data/pdf/36971.pdf)
* [Calvin](http://cs.yale.edu/homes/thomson/publications/calvin-sigmod12.pdf)

### License

Licensed under the Business Source License 1.1 (BUSL-1.1), matching the
`value-rpc` / `raft` dependencies. Copyright (c) 2025-2026 Karagatan LLC.
Change License MPL 2.0 after the Change Date. See [LICENSE](LICENSE).

