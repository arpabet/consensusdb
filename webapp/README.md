# ConsensusDB web apps (Vue 3 + Vite)

**Two** apps built from this one project (multi-page, `base: /`, shared assets),
both calling the admin REST API under `/api`:

- **Dashboard** (`index.html` → `src/dashboard.js` → `DashboardApp.vue`), served
  by the node at **`/`** — read-only monitoring, no mutating actions.
- **Admin console** (`console.html` → `src/admin.js` → `AdminApp.vue`), served at
  **`/console`** — all management, requires an admin sign-in.

Sign-in (`components/Login.vue`, shared) accepts a username + password *or* an IAM
token.

Dashboard view:

- **Dashboard** — cluster/raft status, the live ledger head, per-region footprint
  (keys, size on-transfer and on-disk), and reads/writes per second.

Admin console views:

- **Onboarding** — a first-run wizard (create the admin, optionally generate the
  ledger CA) shown when the cluster needs setup.
- **Access** — users (password login), application tokens (service accounts —
  shown once), and role bindings at instance/tenant/region scope.
- **Nodes** — raft members with up/down health and per-node CPU/memory/storage
  load (storage red over 80%), overall cluster load, add/remove nodes.
- **Database** — export to an encrypted download, or import from a dump file.
- **Verify ledger** — verify a backup against a quorum certificate with a
  **progress bar**.

## Develop

```bash
npm ci
npm run dev        # Vite dev server; proxies /api → http://localhost:8441
```

Point a running node's http-server at `localhost:8441` (the default). Authenticate
in the UI with an IAM token that has `cdb.proofs.read` (e.g. `roles/cdb.auditor`).

## Build & embed

The console is **baked into the server binary** — there is no `webapp/dist` at
runtime. After changing anything here, regenerate the embedded assets from the
repo root:

```bash
make webui          # npm ci && npm run build, then go-bindata → pkg/webui/bindata.go
```

`pkg/webui/bindata.go` is committed, so `go build` stays self-contained; the node
serves the embedded files at `/console` via `pkg/run/spa.go`. Commit the
regenerated `pkg/webui/bindata.go` alongside your front-end changes.

## API used

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET  | `/api/setup/status`        | none  | is first-run setup needed |
| POST | `/api/setup/bootstrap`     | none* | create the first admin (*inert once done) |
| POST | `/api/setup/ledger-ca`     | admin | generate a ledger CA for download |
| GET  | `/api/me`                  | any   | `{principal, isAdmin}` for UI gating |
| GET  | `/api/cluster`             | read  | raft status |
| GET  | `/api/stats`               | read  | cumulative reads/writes + disk size |
| GET  | `/api/regions`             | read  | per-region keys and sizes |
| GET  | `/api/node/metrics`        | read  | this node's CPU/mem/storage (peer fan-out) |
| GET  | `/api/cluster/nodes`       | read  | raft members + health + per-node load |
| POST | `/api/cluster/nodes`       | admin | add a voter (proxied to the leader) |
| DELETE | `/api/cluster/nodes/{id}`| admin | remove a member (proxied to the leader) |
| GET  | `/api/ledger/status`       | read  | current chain head |
| POST | `/api/ledger/verify`       | read  | start a backup-verification job → `{id}` |
| GET  | `/api/ledger/verify/{id}`  | read  | poll job `{state, progress, result, error}` |
| GET  | `/api/database/export`     | admin | stream an (optionally encrypted) dump download |
| POST | `/api/database/import`     | admin | load an uploaded dump |
| GET/POST | `/api/iam/users`        | iam   | list / create users; `DELETE /api/iam/users/{name}` |
| GET/POST | `/api/iam/service-accounts` | iam | list / create (mint token, once); `DELETE …/{name}` |
| GET  | `/api/iam/roles`           | iam   | predefined + custom roles |
| GET/POST | `/api/iam/bindings`     | iam   | list / grant; `POST /api/iam/bindings/revoke` |

Requests (except onboarding) carry an `Authorization` header (Bearer IAM token or
Basic user:password). *read* = `cdb.proofs.read`, *admin* = `cdb.cluster.admin` /
`cdb.backups.*`, *iam* = `cdb.iam.get` (read) / `cdb.iam.set` (write). When
`auth.enabled=false` the apps are open (anonymous).
