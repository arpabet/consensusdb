# consensusdb

ConsensusDB Database

a (near-)linearly scalable shared-nothing database system that
provides high availability without consistency and transactions

primary purpose of this time-based database is immutable data storage for analytics

multiple nodes can write in parallel on local disk, data replicated between nodes by subscription pointers, no leader elected
no minimum nodes setup required, multi-datacenter support by own ssh keys from the list of known

no partitioning of data, all nodes have full data copy, but own write-a-head log with subscribers

# Description

* gRPC interface for database clients and nodes with common CA
* HTTPS REST JSON interface for any clients with common CA
* Engine - badger (similar to rocksdb, but faster)
* Supports point-in-time data by TimeUUID
* Supports compression: Snappy, LZ4
* Supports encryption: AES on GCM, AES on CFB
* Data storage is sealed by default, all payloads are encrypted
* Very fast
* Multi-tenant architecture
* Pure golang implementation

# Current Status
* active developing

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

# Quick start

Build, Run, Write Client

### Prerequisites

Install tools:
```
go get github.com/google/go-licenses
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
open http://localhost:4481/
```


### Run
```
mkdir /tmp/cdb
./consensusdb
./consensusdb  --conf=consensus.yaml

```

### Check

```
lsof -n -i:$PORT | grep LISTEN
```

You have to see that consensusdb is listening 4481 and 4482 ports

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
//  create timeuuid (optional)
//

uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
uuid.SetUnixTimeMillis(1514764800)
uuid.SetMinCounter()

//
//  create key with timeuuid
//

key := cdb.NewKey().WithMajorKey("alex").WithRegionName("ACCOUNT").WithMinorKey("balance").WithTimestamp(uuid).Build()
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

