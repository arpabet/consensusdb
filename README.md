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

* gRPC interface (+ HTTPS REST/JSON gateway) with common CA; a vrpc data plane
  is planned for the store provider
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

# Quick start

Build, Run, Write Client

### Prerequisites

Install tools:
```
go install github.com/google/go-licenses@latest
```

Checkout libs:
```
%GOPATH%\src\github.com\grpc-ecosystem\grpc-gateway v1.14.6
%GOPATH%\src\github.com\protocolbuffers\protobuf v3.12.3
```

Install plugins:
```
%GOPATH%\src\github.com\grpc-ecosystem\grpc-gateway\protoc-gen-grpc-gateway>go install
%GOPATH%\src\github.com\grpc-ecosystem\grpc-gateway\protoc-gen-swagger>go install
```

Generate GRPC stubs from protos: 
```
genproto.bat
```

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

You have to see that consensusdb is listening 8441 and 8442 ports

### Go Client Example

```
//
//  create keychain for data encryption on client side
//

keychain, err := cdb.NewPasswordbasedKeychain("alex")

//
//  create client instance
//

client, err := cdb.NewClient("localhost:4482", keychain)
defer client.Close()

//
//  create uuid (optional)
//

id := uuid.New(uuid.TimebasedVer1)
id.SetUnixTimeMillis(1514764800)
id.SetMinCounter()

//
//  create key with uuid
//

key := cdb.NewKey().WithMajorKey("alex").WithRegionName("ACCOUNT").WithMinorKey("balance").WithTimestamp(id).Build()
value := []byte("1245.90")

//
//  putIfAbsent record with LZ4 compression, AES:CFB encryption with TTL one day and on SLA 100 milliseconds
//

status, err := client.Put(cdb.NewRecord(key, value).UseCompression(cdb.LZ4).UseEncryption(cdb.AES, cdb.CFB).OnlyIfAbsent().WithTtlSeconds(86400).WithTimeout(100))

//
// get record metadata only
//

rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

//
// find the last balance record
//

keyMax := cdb.NewKey().WithMajorKey("alex").WithRegionName("ACCOUNT").WithMinorKey("balance").WithMaxTimestamp().Build()
rec, err := client.GetRecent(cdb.NewRequest(keyMax).HeadOnly())

//
// find 100 messages early or equal key's timestamp
//

rec, err := client.GetRange(cdb.NewRangeRequest(key).WithNumRecords(100))

//
// remove record
//

status, err := client.Remove(cdb.NewRequest(key))

```

### Configuration

Simple configuration example

```
host: localhost
httpPort: 4481
grpcPort: 4482
numCPU: 1
dataDir: /tmp/cdb
```

### Influencers

* [MDCC](http://mdcc.cs.berkeley.edu/)
* [Megastore](https://storage.googleapis.com/pub-tools-public-publication-data/pdf/36971.pdf)
* [Calvin](http://cs.yale.edu/homes/thomson/publications/calvin-sigmod12.pdf)

### License

Licensed under the Business Source License 1.1 (BUSL-1.1), matching the
`value-rpc` / `raft` dependencies. Copyright (c) 2025-2026 Karagatan LLC.
Change License MPL 2.0 after the Change Date. See [LICENSE](LICENSE).

