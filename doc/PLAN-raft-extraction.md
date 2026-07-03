# Plan: standalone `raft` repo + `raftvrpc` (prep before consensusdb Phase 2)

Goal: move the raft modules out of `go.arpabet.com/sprint` into a standalone
`go.arpabet.com/raft` repo, **generalize `raftapi`** so it carries no gRPC types,
**add `raftvrpc`** (value-rpc control plane) **alongside a kept `raftgrpc`**, and
**drop the `sprint` dependency entirely** (custom NodeService etc.), keeping only
servion for serving. Prep for Phase 2, whose follower→leader forwarding is the
control service's `ApplyCommand`.

Refined decisions (2026-07, from owner):
1. **Keep `raftgrpc` and `raftpb`** — other projects may use them. Add `raftvrpc`
   as an additional transport, not a replacement.
2. **Generalize `raftapi`**: remove the gRPC reference. `GetAPIConn` returns
   `any` (each transport's pool returns its own client; callers type-assert).
   Minor breakage in other apps is acceptable.
3. **Move all raft libs into a new repo** `go.arpabet.com/raft`; consensusdb
   depends on it.
4. **Custom `NodeService` (and the other 2-3 needed interfaces); drop `sprint`
   fully.** consensusdb keeps servion for serving only.

---

## Current state (verified 2026-07)

Three transport layers; only two touch gRPC:
1. **Node-to-node raft consensus** — `raftmod/tcp_stream_layer.go` (plain TCP +
   `raft.NetworkTransport`). NOT gRPC. **Unchanged.**
2. **Control service** — `raftpb.RaftService`: Bootstrap/Join/GetConfiguration/
   `ApplyCommand`/`Recover(stream)`. Served by `raftgrpc` over gRPC (kept); a new
   `raftvrpc` serves the same contract over value-rpc.
3. **Client pool** — `raftmod/raft_client_pool.go` dials `*grpc.ClientConn` +
   `grpc_health_v1`. Stays gRPC (paired with raftgrpc); raftvrpc brings its own
   vrpc pool.

gRPC/sprint/raftpb coupling in `raftapi` (the thing to generalize):
- `RaftClientPool.GetAPIConn(addr) (*grpc.ClientConn, error)` → `(any, error)`
  (only real gRPC type in the interface). 3 consumers: the interface, the raftmod
  impl, and one caller in `raftgrpc/raft_api_server.go:138` (leader forwarding).
- `FSMResponse.Status *raftpb.Status` — proto leak; keep for now (raftpb stays),
  revisit if a plain type is cleaner.
- `sprint.Component` / `sprint.Server` embeds — replaced during the sprint drop.

sprint coupling is thin: raftmod only calls `Application.Name()`,
`NodeService.NodeIdHex()`, `NodeService.NodeSeq()` at runtime (per consensusdb's
`sprintshim.go`); the rest is nominal DI. New repo defines those as small local
interfaces.

consensusdb depends only on `raftapi`+`raftmod` today (raftgrpc NOT wired), so
`raftvrpc` is greenfield for it and `ApplyCommand` is the Phase 2 forwarding
primitive.

---

## Target: `go.arpabet.com/raft` (multi-module, go.work)

```
raftapi/     interfaces — NO gRPC types; small local Application/NodeService ifaces
raftpb/      protobuf messages + gRPC service (kept for raftgrpc)
raftgrpc/    gRPC control server + gRPC client pool (kept, adapted to generalized api)
raftvrpc/    value-rpc control server + vrpc client pool (new)
raftmod/     raft node: TCP consensus transport (unchanged), stores, server, serf
```
Deps: glue, hashicorp/raft, raft-badger, store, value + value-rpc, uuid, grpc
(only in raftpb/raftgrpc/raftmod-pool). **No `sprint`.**

---

## Execution order (optimized; honors the 4 priorities)

Rationale for reorder: create `raftvrpc` directly in the new repo (not in sprint
then move), and generalize `raftapi` once. So: generalize in place → move →
add raftvrpc in the new repo → drop sprint.

### S1 — generalize raftapi (in sprint) ✅ DONE (2026-07-02)
- [x] `GetAPIConn` returns `any`; dropped the `google.golang.org/grpc` import from
      `raftapi`. Updated raftmod impl signature and the one raftgrpc caller
      (type-asserts back to `*grpc.ClientConn`). raftapi/raftmod/raftgrpc +
      consensusdb build & test green.

Consumer analysis (de-risks S2): **sprint core does NOT depend on raft\*** — only
`consensusdb` (ours) and `record` (already broken, repoint later) import
`sprint/raft*`. So moving them out of sprint breaks nothing in-workspace. The
"other projects" the owner mentioned are outside this tree; they repoint (minor).
The `raftmod/raftcmd` subpackage (serf CLI) moves with raftmod.

### S2 — move to `go.arpabet.com/raft` ✅ DONE (2026-07-02)
- [x] New repo at `/Users/ashvid/web/arpabet/raft`: relocated raftapi/raftpb/
      raftgrpc/raftmod (+ raftmod/raftcmd), rewrote all `sprint/raft*` imports →
      `raft/*`, go.work + per-module go.mod, top-level Apache-2.0 LICENSE, git init
      + initial commit.
- [x] **Owner decision: DELETED from sprint** — `sprint/raft{api,grpc,mod,pb}`
      removed, sprint go.work cleaned; sprint core builds fine (never depended on
      raft). `record` still imports `sprint/raft*` but is already broken — it
      repoints when revived.
- [x] Inter-module + consensusdb resolution via local `replace` directives (the
      unpublished `raft/*@v1.2.0` versions don't exist yet; go.work alone tried to
      fetch them). Strip replaces at tag time (S5).
- [x] consensusdb repointed (imports + go.mod requires + replaces + go.work);
      build + full suite green incl `-race`. Pure relocation, no behavior change.
      raftmod tests green in the new repo.
- [x] **Finalized on published versions (2026-07-02):** owner released raft
      **v0.2.0** (all submodules) + sprint **v1.2.1**. consensusdb now depends on
      raft/raftapi+raftmod+raftpb @v0.2.0 and sprint @v1.2.1 — all local `replace`
      directives dropped and raft removed from consensusdb go.work. Full suite
      green incl `-race`. No more local-raft coupling.

### S3 — add raftvrpc (new module in the new repo) 🟡 IN PROGRESS (2026-07-02)
- Owner: **raft repo relicensed BUSL-1.1** (matches value-rpc, which raftvrpc
  imports — resolves the Apache/BUSL boundary), README added, raft v0.1.0 +
  sprint v1.2.1 released. value-rpc v1.5.2 / value v1.3.1 are published.
- [x] `raftvrpc` module: codecs (value.Map ↔ raftpb Status/RaftNode/Command/
      RaftConfiguration+RaftServer), `Handler` with the 4 unary ops
      (Bootstrap/Join/GetConfiguration/ApplyCommand) mirroring raftgrpc but with
      valuerpc errors, `Register(srv, handler)`, typed client callers
      (`CallBootstrap`/…), and the `ControlServer` glue bean. ApplyCommand
      leader-forwarding type-asserts `GetAPIConn` → `valueclient.Client`. Auth is
      optional (nil disables the ADMIN gate; mTLS authenticates peers otherwise).
      Build + vet + codec round-trip tests green.
- [x] **vrpc RaftClientPool (DONE)** — `raft_client_pool.go`: `ClientPool`
      implements raftapi.RaftClientPool over valueclient; endpoint = host:(port +
      portDiff) mirroring the grpc pool, cached reconnecting clients, optional
      `*tls.Config` for mTLS parity, lifecycle (PostConstruct/Destroy/Close).
- [x] **CLI (DONE)** — `raft_cmd.go`: `RaftCommand()` (sprint.Command, parallel to
      raftgrpc) dials `raft-vrpc-client.address` and runs config/join/bootstrap via
      the Call* helpers.
- [x] **End-to-end tests (DONE, race-clean)** — `end_to_end_test.go`:
      `TestControlPlaneOverVrpc` drives Bootstrap→GetConfiguration→ApplyCommand over
      an in-memory value-rpc transport against a REAL in-memory raft (test FSM +
      testRaftServer + stub NodeService); `TestClientPoolDialsControlServer` starts
      a real TCP control server, has the `ClientPool` dial it, and drives
      ApplyCommand through the pooled client (the forwarding connection path) +
      checks GetAPIEndpoint port math.
- [ ] **Still deferred**: `Recover` streaming (client-stream via AddIncomingStream,
      snapshot recovery — least critical). Full follower→leader raft forwarding is
      exercised in consensusdb Phase 2 (a 2-node cluster; `raft.Raft` state can't be
      faked, so the mechanism is unit-proven here, integration proven there). mTLS
      pool option wired but not integration-tested (needs certs).

### S4 — custom NodeService, drop sprint
- [ ] Define `Application`/`NodeService`/`PropertyResolver` mini-interfaces in
      raftapi (only the methods used). Replace `sprint.Server`/`sprint.Component`
      embeds with local equivalents.
- [ ] Remove `go.arpabet.com/sprint` from all raft modules.
- [ ] consensusdb: replace `sprintshim.go` with a custom NodeService bean; keep
      servion for serving. Full suite green.

### S5 — tags
- [ ] Tag raft modules; strip local replaces; bump consensusdb; note raftgrpc
      consumers to repoint.

---

## Risks
1. mTLS parity for the vrpc pool (gRPC used TLS creds) — verify in S3.
2. `ApplyCommand` = linearizable forward-to-leader; keep timeout/retry (vrpc
   `resilience` gives retry/circuit-breaker).
3. Moving raftgrpc breaks its import path for other projects — acceptable per
   owner (minor change); consider a deprecated re-export shim in sprint if needed.
4. Don't entangle serf membership in the vrpc migration (consensusdb omits it).
