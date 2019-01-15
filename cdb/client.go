/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
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
	"github.com/shvid/timeuuid"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"math"
	"google.golang.org/grpc"
	"io"
	"context"
	"math/rand"
)

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

var emptyValue = []byte{}
var emptyKey = EmptyKey{}

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
/**
	Key builder
 */

type KeyBuilder struct {
    key *cserverpb.Key
}

func NewKey() KeyBuilder {
	return KeyBuilder { key: &cserverpb.Key{} }
}

func (t KeyBuilder) WithMajorKey(majorKey string) KeyBuilder {
	t.key.MajorKey = []byte(majorKey)
	return t;
}

func (t KeyBuilder) SetMajorKey(majorKey []byte) KeyBuilder {
	t.key.MajorKey = majorKey
	return t;
}

func (t KeyBuilder) MajorKey() []byte {
	return t.key.MajorKey
}

func (t KeyBuilder) WithRegionName(regionName string) KeyBuilder {
	t.key.RegionName = []byte(regionName)
	return t;
}

func (t KeyBuilder) SetRegionName(regionName []byte) KeyBuilder {
	t.key.RegionName = regionName
	return t;
}

func (t KeyBuilder) RegionName() []byte {
	return t.key.RegionName
}

func (t KeyBuilder) WithMinorKey(minorKey string) KeyBuilder {
	t.key.MinorKey = []byte(minorKey)
	return t;
}

func (t KeyBuilder) SetMinorKey(minorKey []byte) KeyBuilder {
	t.key.MinorKey = minorKey
	return t;
}

func (t KeyBuilder) MinorKey() []byte {
	return t.key.MinorKey
}

/**
	Generates random value for the second part of TimeUUID
 */

func (t KeyBuilder) WithTimestamp(timestampMillis int64) KeyBuilder {
	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetUnixTimeMillis(timestampMillis)
	uuid.SetCounter(rand.Int63())
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

/**
	It will calculate SHA1 passwordHash of name (usually it is the context value) and override timestamp in Unix milliseconds to UUID

	Finally we will get unique TimeUUID based o content value and timestamp
 */

func (t KeyBuilder) WithNamedTimestamp(name []byte, timestampMillis int64) KeyBuilder {
	uuid, _ := timeuuid.NameUUIDFromBytes(name, timeuuid.NamebasedVer5)
	// it will override uuid to Time-based UUID
	uuid.SetUnixTimeMillis(timestampMillis)
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) SetTimestamp(uuid timeuuid.UUID) KeyBuilder {
	t.key.Timestamp = &cserverpb.TimeUUID{ MostSigBits: uuid.MostSignificantBits(), LeastSigBits: uuid.LeastSignificantBits() }
	return t;
}

func (t KeyBuilder) Timestamp() timeuuid.UUID {
	return GetTimeUUID(t.key)
}

func GetTimeUUID(key *cserverpb.Key) timeuuid.UUID {
	if key.Timestamp != nil {
		return timeuuid.CreateUUID(key.Timestamp.MostSigBits, key.Timestamp.LeastSigBits)
	} else {
		return timeuuid.Empty
	}
}

func (t KeyBuilder) Build() Key {
	return t
}

func (t KeyBuilder) toProto() *cserverpb.Key {
	return t.key
}

/**
	Key request builder
 */

type KeyRequestBuilder struct {
	request *cserverpb.KeyRequest
}

func NewRequest(key Key) KeyRequestBuilder {
	return KeyRequestBuilder { request: &cserverpb.KeyRequest{ Key: key.toProto() } }
}

func (t KeyRequestBuilder) HeadOnly() KeyRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t KeyRequestBuilder) WithTimeout(timeout int) KeyRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t KeyRequestBuilder) build() *cserverpb.KeyRequest {
	return t.request;
}

/**
	Range request builder
 */

type RangeRequestBuilder struct {
	request *cserverpb.RangeRequest
}

func NewRangeRequest(key Key) RangeRequestBuilder {
	return RangeRequestBuilder { request: &cserverpb.RangeRequest{ Key: key.toProto(), Type: cserverpb.RangeType_LESS_OR_EQUAL, NumRecords: 1 } }
}

func (t RangeRequestBuilder) SetNumRecords(numRecords int) RangeRequestBuilder {
	if numRecords > math.MaxInt32 {
		numRecords = math.MaxInt32
	}
	t.request.NumRecords = int32(numRecords)
	return t;
}

func (t RangeRequestBuilder) HeadOnly() RangeRequestBuilder {
	t.request.HeadOnly = true
	return t;
}

func (t RangeRequestBuilder) WithTimeout(timeout int) RangeRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t RangeRequestBuilder) build() *cserverpb.RangeRequest {
	return t.request;
}

/**
	Record request builder
 */

type RecordRequestBuilder struct {
	key         Key
	request     *cserverpb.RecordRequest
	compressor  Compressor
	cipher      Cipher
	cipherMode  CipherMode
	value       []byte
}

func NewRecord(key Key, value []byte) RecordRequestBuilder {
	return RecordRequestBuilder {
		key:        key,
		request: 	&cserverpb.RecordRequest{ Key: key.toProto(), Metadata: 0 },
		compressor: NO_COMPRESSION,
		cipher: 	NO_ENCRYPTION,
		cipherMode: NO_ENCRYPTION_MODE,
		value: 		value }
}

func NewRecordRequest(key Key) RecordRequestBuilder {
	return RecordRequestBuilder { request: &cserverpb.RecordRequest{ Key: key.toProto() } }
}

func (t RecordRequestBuilder) SetMetadata(metadata int32) RecordRequestBuilder {
	t.request.Metadata = metadata
	return t;
}

func (t RecordRequestBuilder) WithTtlSeconds(ttlSeconds int) RecordRequestBuilder {
	t.request.TtlSeconds = int64(ttlSeconds)
	return t;
}

func (t RecordRequestBuilder) SetTtlSeconds(ttlSeconds int) RecordRequestBuilder {
	t.request.TtlSeconds = int64(ttlSeconds)
	return t;
}

func (t RecordRequestBuilder) OnlyIfAbsent() RecordRequestBuilder {
	t.request.CompareAndSet = true
	t.request.Version 		= 0
	return t;
}

func (t RecordRequestBuilder) CompareAndSet(version uint64) RecordRequestBuilder {
	t.request.CompareAndSet = true
	t.request.Version 		= version
	return t;
}

func (t RecordRequestBuilder) SetValue(value []byte) RecordRequestBuilder {
	t.value = value
	return t;
}

func (t RecordRequestBuilder) UseCompression(compressor Compressor) RecordRequestBuilder {
	t.compressor = compressor
	return t;
}

func (t RecordRequestBuilder) UseEncryption(cipher Cipher, cipherMode CipherMode) RecordRequestBuilder {
	t.cipher = cipher
	t.cipherMode = cipherMode
	return t;
}

func (t RecordRequestBuilder) WithTimeout(timeout int) RecordRequestBuilder {
	if timeout > math.MaxInt32 {
		timeout = math.MaxInt32
	}
	t.request.Timeout = int32(timeout)
	return t;
}

func (t RecordRequestBuilder) build(keychain Keychain) (*cserverpb.RecordRequest, error) {

	value, err := PackValue(t.key, t.value, t.compressor, t.cipher, t.cipherMode, keychain)
	if err != nil {
		return t.request, err
	}

	t.request.Metadata |= t.compressor.MetadataFlag() | t.cipher.MetadataFlag() | t.cipherMode.MetadataFlag()
	t.request.Value = value

	return t.request, nil
}

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

var emptyHead = EmptyHead{}

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

/**
	Record interface
 */

type Record interface {

	Key() Key
	Head() Head

	Exist() bool
	Value() []byte

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

func ParseRecord(record *cserverpb.Record, keychain Keychain) (Record, error) {

	var key Key

	if record.Key != nil {
		key = &KeyBuilder { key: record.Key }
	} else {
		key = emptyKey
	}

	if record.Head != nil {

		compressor, cipher, cipherMode := DecodeMetadata(record.Head.Metadata)
		value, err := UnpackValue(key, record.Value, compressor, cipher, cipherMode, keychain)
		if err != nil {
			return nil, err
		}

		return &RecordResponse{key: key, head: &HeadResponse{record.Head}, exist: true, value: value}, nil

	} else {
		return &RecordResponse{key: key, head: emptyHead, exist: false, value: emptyValue}, nil
	}
	
}

func PackValue(key Key, value []byte, compressor Compressor, cipher Cipher, cipherMode CipherMode, keychain Keychain) (output []byte, err error) {

	output = value

	if compressor != NO_COMPRESSION {
		output, err = compressor.Decompress(output)
		if err != nil {
			return output, err
		}
	}

	if cipher != NO_ENCRYPTION && cipherMode != NO_ENCRYPTION_MODE {
		key, err := keychain.GetBlockKey(key.MajorKey(), key.Timestamp(), cipher.KeyLengthBits())
		if err != nil {
			return output, err
		}
		block, err := cipher.Create(key)
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
		key, err := keychain.GetBlockKey(key.MajorKey(), key.Timestamp(), cipher.KeyLengthBits())
		if err != nil {
			return output, err
		}
		block, err := cipher.Create(key)
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

	Scan(headOnly bool, receiver chan<- Block) error;

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

func (cli *DefaultClient) Scan(headOnly bool, blockC chan<- Block) error {

	defer close(blockC)

	stream, err := cli.kvService.Scan(context.Background(), &cserverpb.ScanRequest{ HeadOnly: headOnly })

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
