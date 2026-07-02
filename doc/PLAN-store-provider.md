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

## Phase 0 — groundwork ✅ DONE (2026-07-02)

- [x] Add `go.work` to consensusdb: `use .` + local `../store`, `../store/providers/badger`,
      `../sprint` (raftapi/raftmod/raftgrpc/raftpb), `../value-rpc`, `../value`.
      Builds and `pkg/...` tests pass against local main-branch checkouts.
- [x] Fix ignored `txn.Commit()` errors in `pkg/server/storage.go` (Put, Remove, Touch) —
      each now returns a wrapped commit error instead of silently dropping it.
- [x] Rewrite `README.md` identity: raft-replicated KV serving the store
      interface (replaced the old leaderless/subscription/analytics framing).

## Phase 1 — versioned envelopes + new raft ops (partially DONE 2026-07-02)

**Architecture correction (supersedes the original bullet).** The plan first said
"replace direct badger usage with injected `store.ManagedDataStore` (badger
provider)". Reading the code shows that is WRONG for determinism: the store
*badger provider* derives its version from badger-native `item.Version()` (a
node-local MVCC commit timestamp), so embedding it would reintroduce exactly the
cross-replica version divergence Phase 1 exists to prevent. It is also a poor fit
— consensusdb's storage is a richer majorKey/region/minorKey/TimeUUID model, not
flat KV.

Chosen instead: **keep consensusdb's own badger storage, but store the store
value *envelope* (`store.EncodeEnvelope` = `version | expiresAt | value`) with
`version = raft entry.Index`, stamped in `FSM.Apply`.** consensusdb depends on
store's envelope *helpers*, not the store engine. This delivers the
raft-index-as-version intent directly, keeps the rich key model, and is far less
invasive. The eventual flat-KV mapping onto store's `RawEntry` lives in the
client provider (Phase 4), not here.

- [x] **Versioned envelopes wired end-to-end (DONE).**
      - `pkg/server/storage.go`: `Put`/`Touch` now take `commitVersion uint64`
        (raft index; 0 = assign locally as `old+1` for raft-off), write
        `EncodeEnvelope(version, expiresAt, value)`, and CAS compares the decoded
        *envelope* version — `item.Version()` deleted from the write path.
      - `FetchRecord` decodes the envelope on every read (version/expiresAt →
        `Head`, payload stripped); `record.go` constructors decoupled from
        `badger.Item` (`RecordHead`/`RecordValue`).
      - `fsm.go` passes `entry.Index`; `server.go` raft-off path passes 0.
      - Tests: `pkg/replication/version_test.go` proves version == raft index,
        cross-replica determinism (B given extra local history so `item.Version()`
        would diverge), and envelope-version CAS. Existing round-trip + snapshot
        tests still green.
      - Known cost (follow-up): `headOnly` reads now copy the value to read the
        17-byte envelope header (the version lives in the value). Acceptable;
        revisit if headOnly scans get hot.
- [x] **TTL determinism (DONE 2026-07-02).**
      - Proto: `RecordRequest.expiresAt` (field 9) and `IncrementRequest.expiresAt`
        (field 6) — absolute unix, computed once on the write-accepting (leader)
        node from `ttlSeconds`; regenerated via `genproto.sh`.
      - `server.go`: the Put/Touch/Increment/Batch handlers call `resolveExpiry`
        (prefers a set `expiresAt`, else `store.ExpiryFromTtl`) BEFORE routing, so
        the command entering the raft log carries a fixed absolute expiry that
        every replica applies identically.
      - **Removed badger's native `entry.ExpiresAt`.** It is NOT a passive GC hint —
        badger hides keys at each node's wall clock, which would desync `Apply`'s
        existence checks across replicas. Expiry now lives solely in the envelope.
      - Read path: `FetchRecord` hides expired entries via `store.IsExpired`
        (returns not-found on point reads; iteration paths skip them), matching
        store's pebble/bbolt lazy-expiry semantics. Existence checks for
        CAS/Increment use raw `txn.Get`, so an expired-but-unswept key still blocks
        putIfAbsent — deterministically, never on a per-node clock.
      - Tests (`ttl_test.go`): cross-replica expiry determinism; expired entry
        hidden on read yet blocks CAS.
      - **Remaining:** physical reclamation of expired envelopes needs a
        DETERMINISTIC sweep (a raft-logged tombstone/delete applied in log order),
        NOT badger's background GC. Until then expired keys are hidden but linger
        on disk. This folds into the watch-hub / sweeper work below.
- [x] **New raft ops `opIncrement` + `opBatch` (DONE 2026-07-02).**
      - Proto: added `IncrementRequest`/`IncrementResponse`/`BatchRequest` messages
        and `Increment`/`Batch` RPCs (with REST gateway routes) to `cdb.proto`;
        regenerated via `genproto.sh` (protoc + plugins present).
      - `storage.go`: `Increment` = envelope read-modify-write of an 8-byte
        big-endian counter (absent ⇒ starts at `Initial`, returns `Previous`,
        matching store's `IncrementRaw` contract) stamped with the raft index;
        `SetBatch` = all records in ONE badger txn (BatchAtomic), each entry
        stamped with the log index. Both take `commitVersion` (0 = local fallback).
      - `command.go` opcodes 4/5 + decode; `fsm.go` apply cases; `fsmResult` gained
        an `incr` field; `Replicator` gained `Increment`/`Batch` (refactored
        `applyCommand` to surface the FSM result); service + `Replicator` interface
        + raft-off direct path all wired.
      - Tests (`increment_test.go`): increment prev/current/version incl. negative
        delta, cross-replica determinism, batch atomic multi-write with per-entry
        log-index version. Full module green.
      - Known limit: `SetBatch` is bounded by badger's max transaction size; very
        large batches would need chunking (which forfeits atomicity) — batches are
        expected to be bounded. `opCAS` as a distinct wire op was NOT added: CAS
        already works through `Put`'s `CompareAndSet` flag.
- [ ] **Determinism — versions (THE load-bearing decision).** The failure to
      prevent: badger's native `Item.Version()` is a node-local MVCC commit
      timestamp; two replicas applying the same log entry get *different*
      `item.Version()` values, so a CAS that reads version V on node A and
      re-checks on node B silently misfires. Current `storage.go` Put compares
      `recordRequest.Version != item.Version()` — that comparison **must be
      deleted**. The invariant that fixes it:

      1. **Version lives in the value envelope, not in the engine.** Every write
         stores `store.EncodeEnvelope(version, expiresAt, value)`; CAS and the
         version returned by Get both read the envelope, never `item.Version()`.
      2. **Version is assigned exactly once, inside `FSM.Apply`, from a
         replicated source.** Apply runs in identical log order on every node, so
         any value it derives there is deterministic. The RPC handler / leader
         must NOT stamp the version (it doesn't know the committed index yet).
      3. **Use the raft `entry.Index` as the version.** It's monotonic,
         cluster-global, unique per committed write, and free (already in hand in
         Apply — no read-before-write for a plain Put). Conformance permits this:
         `storetest` asserts only "version must change on update" (`testVersion`)
         and "the version read back is the one CAS must match" (`testCompareAndSet`);
         it does **not** require per-key `1,2,3…` counting, so a sparse global
         counter is fine.
      4. **Reserve version 0 = absent** (create-if-absent). Raft's first real
         index is 1, so a stored key always has version ≥ 1 and 0 stays
         unambiguous — consistent with storage.go's existing
         `Version==0 ⇒ putIfAbsent`.

      Alternative considered: an envelope per-key counter (`old.version+1`, the
      1,2,3 scheme the bbolt/pebble/mem providers use). Also deterministic in
      Apply and makes cdb byte-identical to those providers, but costs a
      read-before-write on *every* Put to fetch the prior counter. Rejected as
      the default (index is cheaper, equally conformant); revisit only if a
      client depends on dense per-key versions. Single-node / raft-off mode has
      no `entry.Index`, so there it falls back to the envelope per-key counter.
- [ ] **Determinism — TTL.** FSM applies at different wall-clock times per node
      (and again on restart replay), so:
      - The leader computes an **absolute `expiresAt` unix** once and ships it in
        the command; nodes store that exact value. Never a relative ttlSeconds
        (each node's `now+ttl` would diverge). This is what `store.ExpiryFromTtl`
        is for — call it on the leader, before proposing.
      - **`Apply` must not consult wall-clock expiry for existence/version
        decisions.** A CAS(create-if-absent) that treated an expired-but-present
        key as absent on one node and present on another (clock skew at the
        boundary) would diverge state. Rule: Apply decides existence purely from
        envelope presence; expiry is applied only at the **read** layer
        (client-facing Get filters expired) and reclaimed by a **deterministic**
        sweep (a logged tombstone command, or the store sweeper driven on the
        leader and replicated) — never by badger's own background TTL drop, which
        is node-local and non-deterministic. Set badger entry TTL as a
        space-reclaim hint only, never as the source of truth.
- [x] **Watch hub fed from the apply path (DONE 2026-07-02).**
      - `storage.go` embeds a `store.WatchHub` (reused from the store module).
        Each mutation (Put/Touch/Remove/Increment/SetBatch) calls `notify` AFTER a
        successful commit; on the replicated path those methods are driven by
        `FSM.Apply` on every node, so a client watching ANY node sees changes
        committed via raft — **cross-node watch**, the upgrade over the in-process
        embedded providers. Watch is served locally and does NOT go through raft.
      - `WatchRaw(ctx, prefix, cb)` on the storage interface + StorageBean delegates
        to the hub. Event key = encoded entry key (prefix match over the physical
        keyspace); value nil for delete; version = the envelope version (raft index).
      - Proto: `Watch(WatchRequest) returns (stream WatchEvent)` + `ChangeType`
        enum (WATCH_SET/WATCH_DELETE); service handler encodes the prefix Key via
        `EncodeKeyPrefix` (depth inferred from set fields; empty = watch all) and
        decodes each event's key back to `pb.Key`.
      - Tests (`watch_test.go`): mutation-through-FSM delivers a Set event with the
        log-index version; Remove delivers Delete; prefix filter excludes
        non-matching major keys. Race-clean (`go test -race`).
      - NOTE: expiry currently emits NO WatchDelete (lazy hide only). Wiring that up
        is the deterministic-sweep follow-up (a raft-logged tombstone that both
        reclaims disk and notifies) — carried from the TTL bullet.
- [x] Tests: `pkg/replication` version/increment/ttl/watch determinism tests added;
      "two FSMs applying the same log land identical state" covered for versions,
      counters, and expiry. (Multi-node raftgrpc harness is Phase 2.)

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

1. Version determinism (Phase 1) — RESOLVED in the Phase 1 spec: version = raft
   `entry.Index`, stamped once in `FSM.Apply`, stored in the envelope, compared
   from the envelope (never `item.Version()`); 0 reserved = absent. Verified
   conformant against `storetest` (`testVersion` requires only "changes on
   update"; `testCompareAndSet` reads the version back rather than assuming a
   value). This is the one thing that's expensive to change once FSM ops ship, so
   it's nailed down before writing them — do not silently reintroduce
   `item.Version()`.
2. Envelope-in-value vs badger-native metadata: rebasing on the store badger
   provider means native TTL/versions locally but envelope versions for
   replication — make the FSM write path the only writer so the two never mix.
   Corollary of #1: badger `ExpiresAt` and `item.Version()` are space/GC hints
   only, never sources of truth; expiry is enforced at the read layer + a
   deterministic sweep (see Phase 1 TTL bullet).
3. raftgrpc is ported but untested in anger; wiring tests in Phase 2 should
   exercise Bootstrap/Join/leader-transfer on a 3-node in-process harness.
4. Key layout: flat store keys map as majorKey=app/tenant, region="KV",
   minorKey=key. TimeUUID stays empty for the KV use ("record without
   TimeUUID" per the original design). Confirm `EncodeKey` ordering preserves
   lexical order of minorKey within a region (required for `Ordered`).
5. BUSL/Apache boundary — RESOLVED: consensusdb is now Apache-2.0 (2026-07-02),
   matching the store repo; the keychain client can be reused directly.
