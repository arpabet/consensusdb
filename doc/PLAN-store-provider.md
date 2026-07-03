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

## Phase 1 — versioned envelopes + new raft ops ✅ COMPLETE (2026-07-02)

Summary: consensusdb's storage now carries the store value envelope with the raft
log index as a replica-deterministic version; CAS/Increment/Batch are wired as
raft ops; TTL is leader-computed absolute expiry with lazy read-hiding and a
deterministic Reclaimer that emits WatchDelete; a watch hub fed from the apply
path gives cross-node watch. All green incl. `go test -race`. Next: Phase 2
(raftgrpc membership) — the client-facing `store/providers/cdb` is Phase 4.


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
      - Physical reclamation is handled by the deterministic Reclaimer (see the
        sweep bullet below) — DONE.
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
- [x] **Determinism — versions (THE load-bearing decision).** [spec — IMPLEMENTED,
      see "Versioned envelopes wired end-to-end" above; verified: no `item.Version()`
      in the write path, version = `entry.Index`.] The failure to
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
- [x] **Determinism — TTL.** [spec — IMPLEMENTED, see "TTL determinism" above.]
      FSM applies at different wall-clock times per node
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
- [x] **Deterministic expiry sweep / Reclaimer (DONE 2026-07-02) — closes the
      lazy-expiry loop.**
      - `server.Reclaimer` interface (the raft-driven analogue of `store.Sweepable`)
        + `pkg/replication.Reclaimer` bean: a background loop that, on the LEADER
        only, discovers expired entries (`storage.ScanExpired`, a wall-clock scan —
        safe because it merely proposes) and commits their removal through raft
        (`opReclaim` / `pb.ReclaimRequest`). Raft-off mode reclaims directly.
      - `storage.Reclaim` deletes each key ONLY IF its stored envelope version is
        unchanged since discovery (a rewrite/refresh bumps the version → skipped),
        and emits `WatchDelete` per removal. It never re-checks wall-clock expiry at
        apply, so every replica makes the identical decision → deterministic.
      - So expiry now surfaces to watchers as `WatchDelete` and disk is reclaimed,
        without badger's node-local GC. Interval/batch via `reclaim.interval`
        (30s) / `reclaim.batch-size` (1024).
      - Tests (`reclaim_test.go`): scan finds only expired; reclaim removes and
        leaves live entries; version-guard blocks reclaiming a refreshed key;
        WatchDelete emitted; two replicas applying the same Reclaim command converge.
- [x] Tests: `pkg/replication` version/increment/ttl/watch determinism tests added;
      "two FSMs applying the same log land identical state" covered for versions,
      counters, and expiry. (Multi-node raftgrpc harness is Phase 2.)

## Phase 2 — raft cluster completeness

**Updated for raftvrpc (2026-07-02).** The control plane now lives in
`go.arpabet.com/raft/raftvrpc` **v0.3.0** (released; consensusdb pins raft
v0.3.0). raftvrpc gives us `Bootstrap/Join/GetConfiguration/ApplyCommand` over
value-rpc + a `ClientPool` — all tested. Phase 2 wires it into consensusdb and
turns the single-node bootstrap into a real cluster.

Prerequisite discovered: **raftvrpc needs a hosted value-rpc server in
consensusdb.** consensusdb today serves grpc (servion/grpc) + http (servion) but
no vrpc. glue's `FactoryBean` (`Object()/ObjectType()/ObjectName()/Singleton()`)
is the registration pattern — exactly how `servion/grpc/GrpcServerFactory`
publishes a `*grpc.Server` as an injectable bean. This vrpc host is also the
foundation of Phase 3 (vrpc data plane), so build it once here.

Increments (each independently verifiable):
- [x] **2a — vrpc server host (DONE 2026-07-02).** `pkg/replication/vrpc_server.go`:
      `VrpcServerFactory(beanName)` is a `glue.FactoryBean` producing a
      `valueserver.Server` from `<beanName>.bind-address`; `go srv.Run()` in
      `Object()` (registration is concurrent-safe — `AddFunction` uses a sync.Map —
      so handlers register after Run). Empty address ⇒ uniquely-named
      `NewMemServer` fallback (graph resolves, nothing binds). ObjectType =
      `valueserver.Server`.
- [x] **2b — wire the control plane (DONE 2026-07-02).** Added
      `VrpcServerFactory("vrpc-server")`, `raftvrpc.RaftVrpcClientPool()`,
      `raftvrpc.RaftVrpcServer()` to `replication.Beans()`; config
      `vrpc-server.bind-address` (empty=disabled) + `raft.rpc-bean-name=vrpc-server`
      in main.go. Tests: `wiring_test` still green; new `vrpc_wiring_test.go`
      `TestVrpcControlPlaneReachable` binds `tcp://127.0.0.1:0`, dials the control
      endpoint, and confirms Bootstrap dispatches to the handler
      (returns "raft not initialized" with raft off) — end-to-end through the bean
      graph, race-clean. **Deferred**: `raftvrpc.RaftCommand()` is a `sprint.Command`;
      consensusdb uses cligo commands — wire once the command-type compat is checked.
      **License note**: consensusdb (Apache-2.0) now transitively depends on
      value-rpc (BUSL-1.1) via raftvrpc — owner may want to reconcile.
- [x] **2c — membership (DONE 2026-07-03).** `RaftHost` gained a
      `raft.bootstrap` flag (default true): a seed node bootstraps a single-voter
      cluster and becomes leader; joiner nodes set `raft.bootstrap=false` and wait
      to be added by the leader through the control-plane Join RPC (leader-side
      `raft.AddVoter`, which raft replicates to the new node). Doc comment updated.
      Test `membership_test.go` `TestMultiNodeMembershipAndReplication`: a real
      3-node in-memory raft cluster (connected transports, real FSM + storage) —
      bootstrap seed, AddVoter the other two (what Join does), confirm the
      configuration lists 3 servers and a leader write replicates to every node.
      Race-clean.
- [x] **2d — follower write handling (DONE 2026-07-03).** `Replicator.applyCommand`
      now returns a structured `server.NotLeaderError{LeaderID, LeaderAddr}`
      (from `r.LeaderWithID()`) when not leader; `server.AsNotLeader(err)` unwraps
      it. The KeyValueService already propagates the Replicator's error. Test
      `notleader_test.go`: an un-bootstrapped (Follower) node rejects Put and
      Increment with `NotLeaderError`.

      **Design decision — client redirect, NOT server-side forwarding.** Option
      (b), forwarding via `raftvrpc.CallApplyCommand`, was REJECTED: the raft
      control-plane ApplyCommand returns only a `Status`, so it would flatten
      typed responses (Increment's previous/current/version, etc.). Redirecting
      client-side lets the leader return the full typed response directly. The
      client provider (Phase 4) redirects on `NotLeaderError`; conveying the error
      + leader address over the wire (gRPC status details / vrpc error metadata)
      is a Phase 4 concern.
- [x] **2e — multi-node test (mostly DONE 2026-07-03).** The membership +
      replication half is covered by `TestMultiNodeMembershipAndReplication` (2c):
      3-node cluster formed via bootstrap+Join, leader write replicates to all.
      **Remaining, now a Phase 4 concern:** the "kill the leader, a write from a
      follower still commits" scenario is validated *client-side*, because 2d chose
      client-redirect over server-side forwarding — the client provider re-issues
      to the new leader on `NotLeaderError`. So leader-kill/failover-write belongs
      with the Phase 4 provider tests, not a server-side forwarding test.
- [ ] Read consistency: client sticks to the leader for reads+writes
      (read-your-writes after CAS); optional stale follower reads later (Phase 4
      provider option).

## Phase 3 — vrpc data plane

- [x] **Unary vrpc data plane (DONE 2026-07-03).** `pkg/server/vrpc_data.go`:
      `VrpcDataService` bean registers `kv.get/getrecent/put/touch/remove/increment/
      batch` on the shared vrpc host (2a). It's a thin adapter — builds a
      `KeyValueService` over the same injected `Storage`/`Replicator` (avoiding the
      grpc-scanner child-scope issue) so both wires route identically; `set`=put,
      `cas`=put with `RecordRequest.compareAndSet`. value.Map codecs for
      Key/KeyRequest/RecordRequest/Record(+Head)/Status/Increment{Request,Response}/
      BatchRequest; exported `Call*` client helpers. Wired in main.go runScope.
      Test `vrpc_data_test.go` `TestVrpcDataPlaneRoundTrip`: put→get, not-found,
      increment, atomic batch, remove — all over a real tcp vrpc client through the
      full bean graph. Race-clean.
- [x] **Streams: `enumerate` + `watch` (DONE 2026-07-03).** `kv.enumerate`
      (`AddOutgoingStreamTyped`) streams Records under a prefix Key — depth inferred
      from set fields via `areaField`, empty prefix ⇒ Scan; a `chanBlockSender`
      adapts the push-based `BlockSender` to a channel honoring ctx cancellation, so
      credit-based flow control back-pressures the scan. `kv.watch` streams
      `WatchEvent`s from `Storage.WatchRaw` (best-effort, hub drops for slow
      watchers). `watchRequest`/`watchEvent` codecs + `EnumerateStream`/`WatchStream`
      client helpers. Test extends `TestVrpcDataPlaneRoundTrip`: enumerate 3 records
      under a prefix; a put delivers a WATCH_SET event. Stable across `-count=3`,
      race-clean.
      → **Phase 3 data plane complete** (unary + streams). mTLS/token + REST notes
      below stay as ops concerns.
- [ ] Security: mTLS transport + token metadata; reuse servion auth patterns.
- [ ] gRPC `KeyValueService` + REST gateway remain for ops/humans; raftgrpc
      management stays gRPC.

## Phase 4 — `store/providers/cdb` (the client provider)

- [x] **Provider built + working end-to-end (DONE 2026-07-03).** New module
      `go.arpabet.com/store/providers/cdb` (BUSL-1.1) implements `store.DataStore`
      (compile-time asserted). **Decoupled by design**: imports only `store` +
      `value-rpc` — NOT consensusdb — so a client never links badger/raft. It
      speaks the consensusdb vrpc data plane by convention (same `kv.*` function +
      value.Map field names as `VrpcDataService`). Key mapping: flat store key →
      consensusdb `minor` under a fixed major/region; point ops → `Call*`, watch/
      enumerate → streams (client-side prefix/seek filter).
      Integration test in consensusdb (`cdb_provider_test.go`,
      `TestCdbProviderAgainstServer`): a real vrpc server + the provider — SetRaw/
      GetRaw, CAS (correct + stale version), Increment, SetBatch, EnumerateRaw
      (prefix), Remove all round-trip. Race-clean.
- [x] `Features()` = `TTL | Atomic | Watch | BatchAtomic`. **`Ordered` deliberately
      NOT advertised** (see below); `Encrypted` pending keychain; no `Transaction`
      (honest — staphi's CAS/`atomically` fallback covers it).
- [ ] **Remaining — `Ordered` enumeration.** consensusdb's length-prefixed key
      encoding (`[len][bytes]` per field) does NOT preserve lexical order of the
      flat key, so EnumerateRaw yields entries but not sorted; reverse returns
      `ErrNotSupported`. Options: (a) an order-preserving key encoding in
      consensusdb (separator/escaping for the trailing field); (b) client-side
      sort (O(n) per scan). staphi does not need it (it sorts its own results), so
      deferred with a documented design decision.
- [x] **NotLeader redirect (DONE 2026-07-03).** Server: `NotLeaderError` gained
      `LeaderEndpoint`; `Replicator` injects the pool and resolves the leader's
      value-rpc endpoint via `GetAPIEndpoint(leaderRaftAddr)`; `VrpcDataService`
      wraps write handlers (`redirectable`) so a NotLeader becomes a
      `CodeUnavailable` error with message `not-leader:<endpoint>`. Provider:
      `notLeaderEndpoint` detects it, `redirect` redials + **re-sticks to the
      leader** (mutex-guarded client swap), and `call` retries once — callers never
      see the redirect. Reads served by any node. Unit test `cdb_store_test.go`
      covers the error detection; full 2-node failover redirect is the leader-kill
      integration (pairs with the remaining conformance/3-node gate).
      A `README.md` in the provider documents caps, the Ordered decision + the
      opt-in server-side-ordering plan, the redirect, and key mapping.
- [ ] **Remaining — client-side sealing (`Encrypted`).** Integrate the `cdb`
      keychain (AES-GCM). Increment × encryption: server can't add to ciphertext,
      so with sealing on implement `IncrementRaw` as a provider-side CAS loop.
      License is fine now — the provider module is BUSL-1.1, matching value-rpc /
      consensusdb.
- [ ] **Remaining — acceptance gate: run `storetest` conformance** for the
      advertised caps against (a) single node, (b) 3-node cluster w/ leader kill.
      (The manual integration test already exercises the core; conformance is the
      formal gate + benchmark baseline vs embedded badger.)

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
