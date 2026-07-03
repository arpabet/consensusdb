# Plan: retire protobuf/gRPC, adopt the `value` framework, sign records

## Why

consensusdb migrated its transport to **value-rpc**, but its data model is still
**protobuf** (`proto/cdb.proto` → `pkg/pb/*`). The proto messages are load-bearing
(~400 uses across the value-rpc data plane, the raft FSM, storage key encoding),
so they can't just be deleted — but protobuf is now the *only* reason the gRPC
stack, grpc-gateway, and `serviongrpc` are pulled in, and it gives us no record
authenticity.

The `value` framework (already the wire codec via value-rpc) replaces it wholesale:

- `value.PackStruct` / `UnpackStruct` — struct ↔ canonical bytes. `struct.go` sorts
  fields, so the packing is deterministic across processes — **safe as the raft-log
  command encoding** (today that is `proto.Marshal`).
- `value.Marshal` / `Unmarshal` — struct ↔ `value.Map`, replacing the hand-written
  `value.Map`↔`pb.X` codecs in `vrpc_data.go` (they disappear).
- `value.SignBytes(obj, "sign")` — canonical signing projection over `value:"…,sign"`
  fields. Signer and verifier rebuild the same bytes, so they can't drift. This is
  the primitive for **owner-signed records** (proto had nothing here).

Precedent: **depecher** runs its entire comms layer and X3DH/Merkle signing on this
pattern — plain structs with `value:"…"` tags (`common/proto.go`), signatures via
`*SigInput()` → `value.SignBytes` (`common/sigproto.go`).

**On-disk format**: badger data is proto-encoded today. consensusdb is
pre-production (staphi just cut over, single-node deploy), so Phase B assumes a
**fresh cluster** — no on-disk migration shim.

---

## Phase A — retire the gRPC + REST/JSON API (value-rpc is the only API) — ✅ DONE 2026-07-03

Self-contained; proto messages stay. `VrpcDataService` already reuses the same
`KeyValueService` core (`vrpc_data.go:47`), so nothing functional is lost.
All items below done; `go build ./...` + `go test -race ./...` green, `GOWORK=off`
CI build green, `terraform validate` Success. Also removed the legacy in-tree gRPC
client SDK (`cdb/`) and its e2e `main_test.go` (superseded by the value-rpc tests in
`pkg/replication`); the two utils it provided (`CopyOf`, `CreateDirsIfNotExist`)
moved to `pkg/util/fsutil.go`.

- [ ] `proto/cdb.proto`: delete the `service KeyValueService {…}` block and the
      `google/api/annotations` + `protoc-gen-openapiv2` imports/options. Keep every
      `message`/`enum`.
- [ ] Delete `pkg/pb/cdb_grpc.pb.go`, `pkg/pb/cdb.pb.gw.go`, `swagger/`, and the
      now-unused `third_party/` proto imports. Regenerate `cdb.pb.go` (messages only).
- [ ] `genproto.sh`: drop the `--go-grpc_out`, `--grpc-gateway_out`, `--openapiv2_out`
      plugins (keep `--go_out`).
- [ ] `pkg/server/server.go`: drop `pb.UnimplementedKeyValueServiceServer`,
      `RegisterGrpc`, and the gRPC-streaming methods `GetRow`/`GetRegion`/`GetSpace`
      + the `grpc` import. Keep the shared methods value-rpc uses
      (`Get`/`GetRecent`/`Put`/`Touch`/`Remove`/`Increment`/`Batch`).
- [ ] `pkg/run/handlers.go`: delete `GatewayHandler` + `SwaggerHandler`.
- [ ] `main.go`: remove `grpc-server.*` properties, the `serviongrpc.GrpcServerScanner`
      line, and the gateway/swagger handlers from the http-server scanner; drop the
      `serviongrpc` import.
- [ ] `go mod tidy`: drop `serviongrpc`, `grpc`, `grpc-gateway`, `protoc-gen-openapiv2`.
- [ ] Infra: remove the gRPC port (8442) from `Dockerfile` EXPOSE, `infra/statefulset.tf`
      (container/services), and `infra/variables.tf` (`grpc_port`).
- [ ] README: drop the "gRPC interface + REST/JSON gateway" feature line; ports are
      now http (health/metrics) + vrpc data plane.
- [ ] Verify: `go build ./...`, full `-race` test suite, `terraform validate`.

## Phase B — replace `pb.*` messages with `value` structs

- [ ] Define plain Go structs with `value:"…"` tags for every message currently in
      `cdb.proto`: `Key`, `TimeUUID`, `Record`, `RecordRequest`, `KeyRequest`,
      `IncrementRequest`/`Response`, `BatchRequest`, `WatchRequest`/`WatchEvent`,
      `EnumerateRequest`, `ScanRequest`, `RangeRequest`, `Status`, `Head`, `Block`,
      `ReclaimEntry`/`Request`, enums `ChangeType`/`RangeType`. Put them in a new
      `pkg/model` (or keep the `pb` package name to minimise churn).
- [ ] Raft FSM (`pkg/replication/command.go`, `fsm.go`): swap `proto.Marshal`/
      `Unmarshal` for `value.PackStruct`/`UnpackStruct`. **Add a determinism test**:
      pack the same command twice and on a round-trip, assert byte-identical output.
- [ ] value-rpc codecs (`pkg/server/vrpc_data.go`): replace the hand-written
      `keyRequestCodec`/`recordCodec`/… with `value.Marshal`/`Unmarshal` over the new
      structs (large simplification).
- [ ] Key encoding (`pkg/server/key.go`): unchanged logic, retype `pb.Key` → the new
      `Key` struct.
- [ ] Delete `proto/`, `pkg/pb/`, `genproto.sh`, `third_party/`; drop `protobuf` dep.
- [ ] Verify: full `-race` suite (incl. the storetest conformance + the cdb provider
      round-trip/secure/watch tests in `pkg/replication`).

## Phase C — owner-signed records (new capability)

Design pass required — capture decisions before coding:

- [ ] Identity: what key signs a record (data owner / tenant key)? Where do public
      keys live (a `region`/tenant keyring in the store, or supplied per write)?
- [ ] Record shape: add `OwnerSig []byte` + signer id to `Record`; the signed
      projection = canonical `value.SignBytes(record, "sign")` over the value, key
      identity, and version/expiry (domain-separated with a constant `dom` field).
- [ ] Verify policy: verify-on-write at the leader (reject unsigned/invalid) vs.
      store-and-serve-with-flag; interaction with the existing client-side crypto
      middleware (sealing) and with raft (signature travels in the command, verified
      pre-Apply so every replica agrees).
- [ ] Capability: advertise a `Signed` capability on the cdb provider so apps can
      require it.
