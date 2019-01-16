# consensusdb

ConsensusDB Database

a (near-)linearly scalable shared-nothing database system that
provides high availability, strong consistency, and full ACID
transactions

# Description

* gRPC interface for database clients
* HTTP REST JSON interface for any clients
* Engine - badger (similar to rocksdb, but faster)
* Supports point-in-time data by TimeUUID
* Supports compression: Snappy, LZ4
* Supports encryption: AES on GCM, CFB
* Very fast
* Pure golang implementation

# Current Status
* developing a simple one-node version

# Design
Data collocated by majorKey in data nodes, grouped by regionName to reference different types of data, accessible by minorKey and TimeUUID.
All data records are ordered by majorKey/regionName/minorKey/TimeUUID(timestamp, counter)

# Best practices
* Use userId in key.MajorKey, for example "accountNumber", "nickname", "email" or and other primary identifier.
* Use table name in key.RegionName as upper case, for example "ACCOUNT", "PROFILE", "CHAT", "AUTH"
* Use other userId in key.MinorKey with whom we record interraction or type of the event, for example "accountNum", "login", "def"
* Create TimeUUID based on event content and timestamp (this approach is good for MDC)
* Store event with TimeUUID, store record without TimeUUID

# Quick start

### Dependencies

```
go get "gopkg.in/yaml.v2"
go get "github.com/pierrec/lz4"
go get "github.com/golang/snappy"
go get "google.golang.org/grpc"
go get "github.com/grpc-ecosystem/grpc-gateway/runtime"
go get "github.com/golang/protobuf/jsonpb"
```

Init submodules

```
git submodule update --init --recursive
rm -rf vendor/go.etcd.io/etcd/vendor/go.uber.org/zap/
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

