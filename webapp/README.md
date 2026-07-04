# ConsensusDB admin console (Vue 3 + Vite)

The web console for a consensusdb cluster. It calls the admin REST API the node
serves under `/api` and is itself served by the node under `/console`.

Current views:

- **Cluster** — raft state, term, applied index, peers.
- **Ledger head** — the current hash-chain height and digest.
- **Verify a backup** — start a background verification of a backup dump against a
  quorum certificate and watch a **progress bar**; the result shows whether the
  backup is exactly the state a quorum certified.

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

| Method | Path | Purpose |
|---|---|---|
| GET  | `/api/cluster`             | raft status |
| GET  | `/api/ledger/status`       | current chain head |
| POST | `/api/ledger/verify`       | start a backup-verification job → `{id}` |
| GET  | `/api/ledger/verify/{id}`  | poll job `{state, progress, result, error}` |

All requests carry an `Authorization` header (Bearer IAM token or Basic
user:password), authorized by `cdb.proofs.read`.
