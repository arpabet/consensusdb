# ConsensusDB admin console (Vue 3 + Vite)

The web console for a consensusdb cluster. It calls the admin REST API the node
serves under `/api` and is itself served by the node under `/console`.

Current views:

- **Onboarding** — a first-run multi-step wizard (create the admin, choose an auth
  method, generate/download the ledger CA) shown when the cluster needs setup.
- **Dashboard** — cluster/raft status, the live ledger head, per-region footprint
  (keys, size on-transfer and on-disk), and reads/writes per second.
- **Verify ledger** — start a background verification of a backup dump against a
  quorum certificate and watch a **progress bar**; the result shows whether the
  backup is exactly the state a quorum certified.
- **Nodes** (admin only) — raft members with up/down health and per-node
  CPU/memory/storage load (storage red over 80%), overall cluster load, add a node
  (join to raft), and remove a node with a confirmation dialog.
- **Database** (admin only) — export the database to an encrypted download, or
  import from a dump file.

## Develop

```bash
npm ci
npm run dev        # Vite dev server; proxies /api → http://localhost:8441
```

Point a running node's http-server at `localhost:8441` (the default). Authenticate
in the UI with an IAM token that has `cdb.proofs.read` (e.g. `roles/cdb.auditor`).

## Build

```bash
npm run build      # → webapp/dist
```

The node serves `webapp/dist` at `/console` (see `pkg/run/spa.go`; override the
location with the `webapp.dir` property). Production images run `npm ci && npm run
build` and bake `dist` in, or mount it.

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

Requests (except onboarding) carry an `Authorization` header (Bearer IAM token or
Basic user:password). *read* = `cdb.proofs.read`, *admin* = `cdb.cluster.admin` /
`cdb.backups.*`. When `auth.enabled=false` the console is open (anonymous).
