# Plan: consensusdb as the distributed backend for `go.arpabet.com/store`

Goal: staphi (and any store-based app) becomes stateless by swapping its embedded
badger provider for a network provider `store/providers/cdb` backed by a
raft-replicated consensusdb cluster — no application code changes, honest
`Features()` capabilities, client-side sealed encryption preserved.

Decision record: consensusdb chosen over `record` (recordmod is mid sprint→glue
migration and does not build; record is an indexing layer that should later
*consume* the store interface). vrpc carries the client data plane; gRPC stays
for REST gateway + raft management. Main branches everywhere; local `replace` /
`go.work` wiring now, version tags at the end.

Current state (verified 2026-07):
- builds clean, `pkg/replication` tests pass; glue/cligo/servion migration done
- raft core done: FSM + snapshot/restore (`pkg/replication/fsm.go`), raft-badger
  log store via store's badger provider (`pkg/replication/raftstore.go`)
- CAS already on the wire and in storage: `RecordRequest.compare_and_set` +
  `version` (0 = putIfAbsent) honored in `pkg/server/storage.go` Put
- gaps: data path opens badger directly (not `store.ManagedDataStore`); no
  Increment/atomic Batch/Watch; single-node bootstrap only (no join, no
  follower handling); `txn.Commit()` errors ignored in Put/Remove/Touch

---

## Phase 0 — groundwork

- [ ] Add `go.work` to consensusdb: `use .` + local `../store`, `../store/providers/badger`,
      `../sprint` (raftapi/raftmod/raftgrpc/raftpb), `../value-rpc`, `../value`.
- [ ] Fix ignored `txn.Commit()` errors in `pkg/server/storage.go` (Put, Remove, Touch).
- [ ] Rewrite `README.md` identity: raft-replicated KV serving the store
      interface (the current text still describes the old leaderless/subscription
      design and analytics slant).

## Phase 1 — rebase data path onto `store.ManagedDataStore`

The original, uncompleted intent. `KeyValueStorageCtx` (direct badger) becomes a
thin layer over `store.ManagedDataStore` (badger provider). The server then
inherits `IncrementRaw`, `CompareAndSetRaw`, `SetBatchRaw`, `EnumerateRaw`,
`WatchRaw`, `Backup/Restore/DropAll` — everything the store contract needs —
and the FSM opcode switch maps 1:1 onto store raw calls.

- [ ] `pkg/server/storage.go`: replace direct badger usage with injected
      `store.ManagedDataStore`; keep `EncodeKey` mapping
      (majorKey/region/minorKey) as the key layout; snapshot/restore delegates
      to the provider's `Backup`/`Restore`.
- [ ] New raft ops in `pkg/replication/command.go` + `fsm.go`:
      `opCAS`, `opIncrement`, `opBatch` (one log entry = one engine txn →
      `BatchAtomicCapability`), alongside existing opPut/opTouch/opRemove.
- [ ] **Determinism — versions (key design decision):** badger's native
      `Item.Version()` is node-local and diverges across replicas; CAS must
      compare a replica-independent version. Use the raft **log index** as the
      version, carried in the value envelope (`store.EncodeEnvelope`) written by
      the FSM. Deterministic, monotonic, free. Non-replicated (single-node,
      raft off) falls back to the envelope per-key counter.
- [ ] **Determinism — TTL:** FSM applies at different wall-clock times per node;
      commands must carry an **absolute `expiresAt`** computed by the leader,
      never a relative ttlSeconds.
- [ ] Watch: feed a hub from the FSM apply path (the single choke point every
      committed mutation passes through) — reuse `store` watch-hub machinery.
      This yields **cross-process watch**, an upgrade over every embedded provider.
- [ ] Tests: extend `pkg/replication` wiring/fsm tests for the new ops;
      determinism test = two FSMs applying the same log land identical state.

## Phase 2 — raft cluster completeness

- [ ] Wire `go.arpabet.com/sprint/raftgrpc` (already ported locally, 4 files —
      the missing management piece identified in record's MIGRATION.md):
      `RaftGrpcServer()` bean into the grpc-server scanner in `main.go`,
      `RaftCommand()` into the cligo commands. Local replace; tag later.
- [ ] Membership: Bootstrap/Join via raftgrpc replaces the single-voter-only
      `RaftHost` bootstrap (`pkg/replication/host.go`); serf optional, later.
- [ ] Follower handling, staged:
      (a) followers reject writes with NotLeader + leader address hint;
          the client provider redirects and re-sticks to the leader;
      (b) transparent server-side forwarding later if needed.
- [ ] Read consistency: the client sticks to the leader for reads and writes
      (read-your-writes preserved — required by the store contract after CAS).
      Optional stale follower reads as an explicit provider option, later.

## Phase 3 — vrpc data plane

- [ ] New vrpc endpoint alongside gRPC (both are thin adapters over the same
      `StorageBean`/`Replicator` beans): unary `get/set/cas/increment/touch/
      remove/batch`, server-stream `enumerate` and `watch` (credit-based flow
      control fits both; watch stays honest best-effort). Payloads are
      `value.Map` — `store.RawEntry` maps 1:1, same MessagePack family as
      store's `codec_msgpack.go`.
- [ ] Security: mTLS transport + token metadata; reuse servion auth patterns.
- [ ] gRPC `KeyValueService` + REST gateway remain for ops/humans; raftgrpc
      management stays gRPC.

## Phase 4 — `store/providers/cdb` (the client provider)

- [ ] New module in the store monorepo implementing `DataStore` (+ manager ops
      where the server exposes them), speaking vrpc, with leader discovery and
      `value-rpc/resilience` retry/circuit-breaker on NotLeader/Unavailable.
- [ ] `Features()`: `TTL | Atomic | Ordered | Watch | BatchAtomic`
      (+ `Encrypted` when a keychain is configured). **No** `Transaction` —
      honest per the capability contract; staphi's CAS-based uniqueness and
      `atomically` fallback were designed for exactly this profile.
- [ ] Client-side sealing: integrate the `cdb` keychain (AES-GCM, per-tenant
      block keys) as a provider option. **Increment × encryption conflict**: the
      server cannot add to ciphertext — when sealing is on, implement
      `IncrementRaw` as a provider-side CAS retry loop (still linearizable per
      attempt; extra round-trip only under contention).
- [ ] License note: RESOLVED — consensusdb was relicensed BUSL-1.1 → Apache-2.0
      (2026-07-02), so the provider can freely reuse the `cdb` keychain client
      and vrpc message surface with no clean-room or cross-license concern.
- [ ] **Acceptance gate: `storetest` conformance suite passes** for the
      advertised capability set, against (a) single node, (b) 3-node cluster
      with leader kill mid-run. Add to the benchmarks runner for a latency
      baseline vs embedded badger.

## Phase 5 — staphi proof

- [ ] Provider selection by env in `main.go` (badger dir vs cdb address) —
      the only staphi change.
- [ ] Run 2 stateless staphi replicas against 1- and 3-node consensusdb; the
      concurrent-registration regression test
      (`server/stores_test.go`) must pass across replicas.
- [ ] Later: replace staphi's in-process `Broker` with store `Watch` (now
      cross-process), removing the last single-node assumption.

## Phase 6 — versioning & release (deferred by design)

- [ ] Tag `sprint/raftgrpc`, consensusdb, `store/providers/cdb`; strip
      local replaces; per-repo release notes. (User will sort versions later —
      everything until here rides main branches.)

## Risks / open questions

1. Version determinism (Phase 1) is the one decision that's expensive to change
   later — settle raft-index-as-version before writing the FSM ops.
2. Envelope-in-value vs badger-native metadata: rebasing on the store badger
   provider means native TTL/versions locally but envelope versions for
   replication — make the FSM write path the only writer so the two never mix.
3. raftgrpc is ported but untested in anger; wiring tests in Phase 2 should
   exercise Bootstrap/Join/leader-transfer on a 3-node in-process harness.
4. Key layout: flat store keys map as majorKey=app/tenant, region="KV",
   minorKey=key. TimeUUID stays empty for the KV use ("record without
   TimeUUID" per the original design). Confirm `EncodeKey` ordering preserves
   lexical order of minorKey within a region (required for `Ordered`).
5. BUSL/Apache boundary — RESOLVED: consensusdb is now Apache-2.0 (2026-07-02),
   matching the store repo; the keychain client can be reused directly.
