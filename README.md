# bigbagger

BigBagger Database

This project started on Halloween!

# Description

* gRPC interface for database clients
* HTTP REST JSON interface for any clients
* For now works as a single data node, but in future I will add cluster support through etcd
* Engine - badger (similar to rocksdb, but faster)
* Supports point-in-time data
* Supports compression
* Supports encryption
* Very fast

# Quick start

### Dependencies

```
go get "gopkg.in/ini.v1"
go get "github.com/dsnet/compress/bzip2"
go get "github.com/pierrec/lz4"
go get "github.com/gobwas/glob"
go get "google.golang.org/grpc"
go get "github.com/grpc-ecosystem/grpc-gateway/runtime"
go get "github.com/golang/protobuf/jsonpb"
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
mkdir /tmp/bigbagger
./bigbagger
curl -d "@create.json" -H "Content-Type: application/json" -X POST http://localhost:4481/v1/table
```

### Check

```
lsof -n -i:$PORT | grep LISTEN
```

You have to see that bigbagger is listening 4481 and 4482 ports

### Go Client Example

```
//
// connect to BigBagger server
//

client, err := bbclient.NewClient(grpcAddress)
defer client.Close()

//
// Create Region TEST
//

region := new(bbproto.Region)
region.Version = "1.0"
region.Name = "TEST"
region.Ttl = "1D"     // one day

err = client.CreateRegion(region)

//
// Put
//

op = bbclient.Put("TEST", []byte("key"), []byte("value")).OverrideTtl(1000)

res = client.Execute(op)

//
// Get
//

op = bbclient.Get("TEST", []byte("key"))

res = client.Execute(op)

if res.IsError() {
    fmt.Print("get error: ", res.GetError())
    return
}

data := res.GetRecord().Value()

// Put PIT Value

op = bbclient.Put("TEST", []byte("key"), []byte("value")).WithTimestamp(1514764800)

res = client.Execute(op)

```

// Get Last PIT Value

op = bbclient.Range("TEST", []byte("key"), 1).WithTimestamp(math.MaxUint64)

res = client.Execute(op)

if res.Exists() {
   data = res.GetRecord().Value()
}

### Bagger

Bagger - is the touring motorcycle for long trips (for large baggage). BMW K1600B for example.

