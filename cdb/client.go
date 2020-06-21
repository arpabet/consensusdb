/*
 *
 * Copyright 2020-present Arpabet, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */


package cdb

import (
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"google.golang.org/grpc"
	"io"
	"context"
	"github.com/consensusdb/timeuuid"
	"fmt"
	"github.com/golang/protobuf/proto"
)

var (

  emptyValue = []byte{}
  emptyKey = EmptyKey{}
  emptyHead = EmptyHead{}

)


/**
	Status
 */

type Status interface {
	Updated() bool
}

type StatusResponse struct {
	status  *cserverpb.Status
}

func (t StatusResponse) Updated() bool {
	return t.status.Updated
}
func (t StatusResponse) String() string {
	if t.status.Updated {
		return "Updated"
	} else {
		return "NotUpdated"
	}
}

/**
	Key interface
 */

type Key interface {

	MajorKey()   []byte
	RegionName() []byte
	MinorKey()   []byte
	Timestamp()  timeuuid.UUID

	toProto()  *cserverpb.Key

}

type EmptyKey struct {
}

func (t EmptyKey) MajorKey() []byte {
	return emptyValue
}

func (t EmptyKey) RegionName()  []byte {
	return emptyValue
}

func (t EmptyKey) MinorKey()   []byte {
	return emptyValue
}

func (t EmptyKey) Timestamp()  timeuuid.UUID {
	return timeuuid.Empty
}

func (t EmptyKey) toProto()  *cserverpb.Key {
	return new(cserverpb.Key)
}

func (t EmptyKey) String() string {
	return ""
}


/**
	Head interface
 */

type Head interface {

	Version()    uint64
	ExpiresAt()  uint64
	DiskSize()   int64
	Metadata()   int32

}

type EmptyHead struct {
}

func (t EmptyHead) Version() uint64 {
	return 0
}

func (t EmptyHead) ExpiresAt() uint64 {
	return 0
}

func (t EmptyHead) DiskSize() int64 {
	return 0
}

func (t EmptyHead) Metadata() int32 {
	return 0
}

func (t EmptyHead) String() string {
	return ""
}

type HeadResponse struct {
	head   *cserverpb.Head
}

func (t HeadResponse) Version() uint64 {
	return t.head.Version
}

func (t HeadResponse) ExpiresAt() uint64 {
	return t.head.ExpiresAt
}

func (t HeadResponse) DiskSize() int64 {
	return t.head.DiskSize
}

func (t HeadResponse) Metadata() int32 {
	return t.head.Metadata
}

func (t HeadResponse) String() string {
	return fmt.Sprint(t.head)
}

/**
	Record interface
 */

type Record interface {

	Key() Key
	Head() Head

	Exist() bool
	Value() []byte

	ParseTo(pb proto.Message) error

}

type RecordResponse struct {

	key Key
	head Head
	value []byte
	exist bool

}

func (t RecordResponse) Key() Key {
	return t.key
}

func (t RecordResponse) Head() Head {
	return t.head
}

func (t RecordResponse) Value() []byte {
	return t.value
}

func (t RecordResponse) Exist() bool {
	return t.exist
}

func (t RecordResponse) ParseTo(pb proto.Message) error {
	if len(t.value) > 0 {
		err := proto.Unmarshal(t.value, pb)
		if err != nil {
			return err
		}
	} else {
		pb.Reset()
	}
	return nil
}

func (t RecordResponse) String() string {
	return fmt.Sprint(t.key, "=value[", len(t.value), "] (", t.head, ")")
}

func ParseRecord(record *cserverpb.Record, keychain Keychain) (rec Record, err error) {

	var key Key

	if record.Key != nil {
		key = &KeyBuilder { key: record.Key }
	} else {
		key = emptyKey
	}

	if record.Head != nil {

		value := emptyValue
		if len(record.Value) != 0 {
			compressor, cipher, cipherMode := DecodeMetadata(record.Head.Metadata)
			value, err = UnpackValue(key, record.Value, compressor, cipher, cipherMode, keychain)
			if err != nil {
				return nil, err
			}
		}

		return &RecordResponse{key: key, head: &HeadResponse{record.Head}, exist: true, value: value}, nil

	} else {
		return &RecordResponse{key: key, head: emptyHead, exist: false, value: emptyValue}, nil
	}
	
}

func PackValue(key Key, value []byte, compressor Compressor, cipher Cipher, cipherMode CipherMode, keychain Keychain) (output []byte, err error) {

	output = value

	if compressor != NO_COMPRESSION {
		output, err = compressor.Compress(output)
		if err != nil {
			return output, err
		}
	}

	if cipher != NO_ENCRYPTION && cipherMode != NO_ENCRYPTION_MODE {
		blockKey, err := keychain.GetBlockKey(key.MajorKey(), key.Timestamp(), cipher.KeyLengthBits())
		if err != nil {
			return output, err
		}
		defer blockKey.clear()
		block, err := cipher.Create(blockKey)
		if err != nil {
			return output, err
		}
		output, err = cipherMode.Encrypt(block, output)
		if err != nil {
			return output, err
		}
	}

	return output, nil
}

func UnpackValue(key Key, value []byte, compressor Compressor, cipher Cipher, cipherMode CipherMode, keychain Keychain) (output []byte, err error) {

	output = value

	if cipher != NO_ENCRYPTION && cipherMode != NO_ENCRYPTION_MODE {

		blockKey, err := keychain.GetBlockKey(key.MajorKey(), key.Timestamp(), cipher.KeyLengthBits())
		if err != nil {
			return output, err
		}
		defer blockKey.clear()

		block, err := cipher.Create(blockKey)
		if err != nil {
			return output, err
		}
		output, err = cipherMode.Decrypt(block, output)
		if err != nil {
			return output, err
		}
	}

	if compressor != NO_COMPRESSION {
		output, err = compressor.Decompress(output)
		if err != nil {
			return output, err
		}
	}

	return output, nil
}

func DecodeMetadata(metadata int32) (Compressor, Cipher, CipherMode) {

	var cipher Cipher
	var cipherMode CipherMode

	if metadata & bitAES > 0 {
		cipher = AES
	} else {
		cipher = NO_ENCRYPTION
	}

	if metadata & bitGCM > 0 {
		cipherMode = GCM
	} else if metadata & bitCFB > 0 {
		cipherMode = CFB
	} else {
		cipherMode = NO_ENCRYPTION_MODE
	}

	var compressor Compressor

	if metadata & bitLZ4 > 0 {
		compressor = LZ4
	} else if metadata & bitSnappy > 0 {
		compressor = SNAPPY
	} else {
		compressor = NO_COMPRESSION
	}

	return compressor, cipher, cipherMode
}


/**
	Block is the array of Records
 */

 type Block []Record


 func ParseBlock(block *cserverpb.Block, keychain Keychain) (Block, error) {

 	if block.Record != nil {

 		size := len(block.Record)
 		response := make([]Record, 0, size)

 		for i := 0; i < size; i = i + 1 {
			block, err := ParseRecord(block.Record[i], keychain)
			if err != nil {
				return nil, err
			}
 			response = append(response, block)
		}

		return response, nil

	} else {
		return []Record{}, nil
	}

 }

type Client interface {

	Get(KeyRequestBuilder) (Record, error);

	GetRecent(KeyRequestBuilder) (Record, error);

	GetRange(RangeRequestBuilder) (Block, error);

	GetRow(KeyRequestBuilder, chan<- Block) error;

	GetRegion(KeyRequestBuilder, chan<- Block) error;

	GetSpace(KeyRequestBuilder, chan<- Block) error;

	Scan(ScanRequestBuilder, chan<- Block) error;

	Touch(RecordRequestBuilder) (Status, error);

	Put(RecordRequestBuilder) (Status, error);

	Remove(KeyRequestBuilder) (Status, error);

}

type DefaultClient struct {
	conn               *grpc.ClientConn
	kvService 		   cserverpb.KeyValueServiceClient
	keychain           Keychain
}

func (cli *DefaultClient) Close() error {
	if conn := cli.conn; conn != nil {
		cli.conn = nil
		return conn.Close()
	}
	return nil
}

func NewClient(grpcAddress string, keychain Keychain) (*DefaultClient, error) {

	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	var cli = &DefaultClient{conn, cserverpb.NewKeyValueServiceClient(conn), keychain}
	return cli, nil
}

func (cli *DefaultClient) Get(builder KeyRequestBuilder) (Record, error) {
	rec, err := cli.kvService.Get(context.Background(), builder.build())
	if err != nil {
		return nil, err
	}
	return ParseRecord(rec, cli.keychain)
}

func (cli *DefaultClient) GetRecent(builder KeyRequestBuilder) (Record, error) {
	rec, err :=  cli.kvService.GetRecent(context.Background(), builder.build())
	if err != nil {
		return nil, err
	}
	return ParseRecord(rec, cli.keychain)
}

func (cli *DefaultClient) GetRange(builder RangeRequestBuilder) (Block, error) {
	block, err := cli.kvService.GetRange(context.Background(), builder.build())
	if err != nil {
		return nil, err
	}
	return ParseBlock(block, cli.keychain)
}

type blockReceiver interface {

	Recv() (*cserverpb.Block, error)

}

func (cli *DefaultClient) streamLoop(receiver blockReceiver, blockC chan<- Block) error {

	for {

		block, err := receiver.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		outBlock, err := ParseBlock(block, cli.keychain)
		if err != nil {
			return err
		}

		blockC <- outBlock

	}

}

func (cli *DefaultClient) GetRow(builder KeyRequestBuilder, blockC chan<- Block) error {

	defer close(blockC)

	stream, err := cli.kvService.GetRow(context.Background(), builder.build())
	if err != nil {
		return err
	}

	return cli.streamLoop(stream, blockC)
}

func (cli *DefaultClient) GetRegion(builder KeyRequestBuilder, blockC chan<- Block) error {

	defer close(blockC)

	stream, err := cli.kvService.GetRegion(context.Background(), builder.build())
	if err != nil {
		return err
	}

	return cli.streamLoop(stream, blockC)
}

func (cli *DefaultClient) GetSpace(builder KeyRequestBuilder, blockC chan<- Block) error {

	defer close(blockC)

	stream, err := cli.kvService.GetSpace(context.Background(), builder.build())
	if err != nil {
		return err
	}

	return cli.streamLoop(stream, blockC)
}

func (cli *DefaultClient) Scan(builder ScanRequestBuilder, blockC chan<- Block) error {

	defer close(blockC)

	stream, err := cli.kvService.Scan(context.Background(), builder.build())
	if err != nil {
		return err
	}

	return cli.streamLoop(stream, blockC)
}

func (cli *DefaultClient) Touch(builder RecordRequestBuilder) (Status, error) {

	request, err := builder.build(cli.keychain)
	if err != nil {
		return nil, err
	}

	status, err := cli.kvService.Touch(context.Background(), request)

	return &StatusResponse{ status }, err
}

func (cli *DefaultClient) Put(builder RecordRequestBuilder) (Status, error) {

	request, err := builder.build(cli.keychain)
	if err != nil {
		return nil, err
	}

	status, err := cli.kvService.Put(context.Background(), request)

	return &StatusResponse{ status }, err
}

func (cli *DefaultClient) Remove(builder KeyRequestBuilder) (Status, error) {
	status, err := cli.kvService.Remove(context.Background(), builder.build())
	return &StatusResponse{ status }, err
}
