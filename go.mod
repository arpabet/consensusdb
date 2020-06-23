module github.com/consensusdb/consensusdb

go 1.13

require github.com/pkg/errors v0.9.1

require github.com/dgraph-io/badger/v2 v2.0.3

require google.golang.org/grpc v1.29.1

require github.com/grpc-ecosystem/grpc-gateway v1.14.6

require gopkg.in/yaml.v2 v2.3.0

require github.com/golang/protobuf v1.4.2

require github.com/golang/snappy v0.0.1

require go.uber.org/atomic v1.6.0

require go.uber.org/zap v1.15.0

require (
	github.com/consensusdb/timeuuid v0.0.0-20200621080525-86214e0daa08
	github.com/frankban/quicktest v1.9.0 // indirect
	github.com/pierrec/lz4 v2.5.2+incompatible
	github.com/prometheus/client_golang v1.7.0
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	google.golang.org/genproto v0.0.0-20200513103714-09dca8ec2884
	google.golang.org/protobuf v1.23.0
)
