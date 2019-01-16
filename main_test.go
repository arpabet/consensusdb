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

package main_test

import (
	"testing"
	"io/ioutil"
	"github.com/consensusdb/consensusdb/cserver"
	"time"
	"github.com/consensusdb/consensusdb/cdb"
	"os"
	"bytes"
	"log"
	"math/rand"
	"github.com/shvid/timeuuid"
	"fmt"
)


func TestSuit(t *testing.T) {

	//
	//  Create Payload
	//

	payload := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		payload[i] = byte(i)
	}

	println("DatasetTest executed")

	keychain, err := cdb.NewPasswordbasedKeychain("alex")
	if err != nil {
		t.Fatal("fail to create keychain", err)
	}

	rootDir, err := ioutil.TempDir("/tmp", "consensusdb_test")

	if err != nil {
		t.Fatal("fail to create tmp dir ", err)
	}

	defer os.RemoveAll(rootDir)

	conf, err := cserver.NewDefaultConfiguration(rootDir)
	if err != nil {
		t.Fatal("fail to create configuration", err)
	}

	server, err := cserver.NewServer(conf)
	defer server.Close()

	if err != nil {
		t.Fatal("fail to create a cserver ", err)
		return
	}

	go server.ServeGRPC()

	time.Sleep(time.Second)

	client, err := cdb.NewClient(conf.GrpcAddress, keychain)
	defer client.Close()

	if err != nil {
		t.Fatal("fail to create a cdb ", err)
	}

	regionName := "TEST"

	RunCRUIDTests(t, client, regionName)
	RunCompareAndSetTests(t, client, regionName)
	RunWithTtlTests(t, client, regionName)


	for _, c := range cdb.KnownCompressors {

		RunCompressionTests(t, client, regionName, c, []byte{})
		RunCompressionTests(t, client, regionName, c, []byte("a"))
		RunCompressionTests(t, client, regionName, c,  payload)

		for _, cipher := range cdb.KnownCiphers {

			for _, mode := range cdb.KnownCipherModes {

				RunEncryptionTests(t, client, c, cipher, mode, regionName, []byte{})
				RunEncryptionTests(t, client, c, cipher, mode, regionName, []byte("a"))
				RunEncryptionTests(t, client, c, cipher, mode, regionName, payload)
			}

		}
	}

	RunEncryptionTests(t, client, cdb.NO_COMPRESSION, cdb.AES, cdb.CFB, regionName, []byte{})
	RunEncryptionTests(t, client, cdb.NO_COMPRESSION, cdb.AES, cdb.CFB, regionName, []byte("a"))
	RunEncryptionTests(t, client, cdb.NO_COMPRESSION, cdb.AES, cdb.CFB, regionName, payload)

	RunPitOneTests(t, client, regionName)
	RunSpaceTests(t, client, "CHAT")

}

func RunCRUIDTests(t *testing.T, client cdb.Client, regionName string) {

	defValue := []byte("value")

	//
	//  Test Not Exists
	//

	key := cdb.NewKey().WithMajorKey("cruid").WithRegionName(regionName).WithMinorKey("def")

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	log.Println("rec=", rec)

	if rec.Exist() {
		t.Fatal("this is the new test, entry must not exist")
	}

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, defValue))

	if err != nil {
		t.Fatal("put failed")
	}

	if !status.Updated() {
		t.Fatal("entry must be updated")
	}

	//
	//  Test Exists
	//

	rec, err = client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must exists")
	}

	//
	//  Test Get
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must exists")
	}

	data := rec.Value()

	if !bytes.Equal(defValue, data) {
		t.Fatal("wrong data found")
	}

	//
	//  Test Remove
	//

	status, err = client.Remove(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("remove failed")
	}

	if !status.Updated() {
		t.Fatal("must update entry on remove")
	}

	//
	//  Test Not Exists
	//

	rec, err = client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if rec.Exist() {
		t.Fatal("entry must be removed")
	}


}


func RunCompareAndSetTests(t *testing.T, client cdb.Client, regionName string) {

	var majorKey [16]byte
	rand.Read(majorKey[:])

	key := cdb.NewKey().SetMajorKey(majorKey[:]).WithRegionName(regionName).WithMinorKey("def")

	//
	//  Test Not Exists
	//

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if rec.Exist() {
		t.Fatal("this is the new test, entry must not exist")
	}

	//
	//  Test Put If Absent
	//

	firstValue := []byte("first")

	status, err := client.Put(cdb.NewRecord(key, firstValue).OnlyIfAbsent())

	if err != nil {
		t.Fatal("putIfAbsent failed")
	}

	if !status.Updated() {
		t.Fatal("putIfAbsent must update record")
	}

	//
	//  Test Get First
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if !bytes.Equal(firstValue, rec.Value()) {
		t.Fatal("wrong value returned")
	}

	firstVersion := rec.Head().Version()

	if firstVersion <= 0 {
		t.Fatal("wrong first version")
	}

	//
	//  Test Replace
	//

	secondValue := []byte("second")

	status, err = client.Put(cdb.NewRecord(key, secondValue).CompareAndSet(firstVersion))

	if err != nil {
		t.Fatal("replace failed")
	}

	if !status.Updated() {
		t.Fatal("compareAndSet not triggered")
	}

	//
	//  Test Get Second
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if !bytes.Equal(secondValue, rec.Value()) {
		t.Fatal("wrong value returned")
	}

	secondVersion := rec.Head().Version()

	if secondVersion <= firstVersion {
		t.Fatal("wrong value of the second version")
	}

	//fmt.Print("secondVersion=", secondVersion, "\n")

	//
	//  Test Remove
	//

	status, err = client.Remove(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("remove failed")
	}

	if !status.Updated() {
		t.Fatal("expected updated entry on remove")
	}


}


func RunWithTtlTests(t *testing.T, client cdb.Client, regionName string) {

	key := cdb.NewKey().WithMajorKey("ttl").WithRegionName(regionName).WithMinorKey("def").Build()

	//
	//  Test Not Exists
	//

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if rec.Exist() {
		t.Fatal("this is the new test, entry must not exist")
	}

	if rec.Head().ExpiresAt() != 0 {
		t.Fatal("expected zero for expiration time")
	}

	if rec.Head().Version() != 0 {
		t.Fatal("expected zero for version")
	}

	if rec.Head().DiskSize() != 0 {
		t.Fatal("expected zero disk size")
	}

	if rec.Head().Metadata() != 0 {
		t.Fatal("expected zero metadata")
	}

	//
	//  Test Put With TTL
	//

	defValue := []byte("value")

	status, err := client.Put(cdb.NewRecord(key, defValue).SetTtlSeconds(100))

	if err != nil {
		t.Fatal("put failed")
	}

	if !status.Updated() {
		t.Fatal("entry must be updated")
	}

	//
	//  Test Exists With TTL
	//

	rec, err = client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("value with ttl not found")
	}

	//fmt.Print("ExpireAt=", res.GetExpiresAt(), "\n")

	if rec.Head().ExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time")
	}

	if rec.Head().Version() == 0 {
		t.Fatal("expected non zero for version time")
	}

	if rec.Head().DiskSize() == 0 {
		t.Fatal("expected non zero for disk size")
	}

	//
	//  Test Get With TTL
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("value with ttl not found")
	}

	if !bytes.Equal(defValue, rec.Value()) {
		t.Fatal("wrong value with ttl")
	}

	firstExpiresAt := rec.Head().ExpiresAt()

	//
	//  Test Touch
	//

	status, err = client.Touch(cdb.NewRecordRequest(key).WithTtlSeconds(1000))

	if err != nil {
		t.Fatal("touch failed")
	}

	if !status.Updated() {
		t.Fatal("touch did not update result")
	}

	//
	//  Check new TTL
	//

	rec, err = client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if firstExpiresAt >= rec.Head().ExpiresAt() {
		t.Fatal("after touch expire at time must be changed")
	}

}


func RunCompressionTests(t *testing.T, client cdb.Client, regionName string, compressor cdb.Compressor, payload []byte) {

	key := cdb.NewKey().WithMajorKey("compression").WithRegionName(regionName).WithMinorKey("def").Build()

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, payload).UseCompression(compressor))

	if err != nil {
		t.Fatal("fail to put entry: ", err)
	}

	if !status.Updated() {
		t.Fatal("entry not updated")
	}

	//
	//  Test Size
	//

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry not found")
	}

	if rec.Head().DiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	//
	//  Test Get
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if rec.Head().DiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	if !bytes.Equal(payload, rec.Value()) {
		t.Fatal("actual value is not the same as payload")
	}

}


func RunEncryptionTests(t *testing.T, client cdb.Client, compressor cdb.Compressor, cipher cdb.Cipher, cipherMode cdb.CipherMode, regionName string, payload []byte) {

	key := cdb.NewKey().WithMajorKey("encryption").WithRegionName(regionName).WithMinorKey("def").Build()

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, payload).UseCompression(compressor).UseEncryption(cipher, cipherMode))

	if err != nil {
		t.Fatal("put failed")
	}

	if !status.Updated() {
		t.Fatal("entry must be updated")
	}

	//
	//  Test Size
	//

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed", err)
	}

	if !rec.Exist() {
		t.Fatal("entry not found")
	}

	if rec.Head().DiskSize() <= 1 {
		t.Fatal("wrong entry size")
	}

	//
	//  Test Get
	//

	rec, err = client.Get(cdb.NewRequest(key))

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry not found")
	}

	if !bytes.Equal(payload, rec.Value()) {
		t.Fatal("actual value is wrong")
	}

}


func RunPitOneTests(t *testing.T, client cdb.Client, regionName string) {

	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetUnixTimeMillis(1514764800)
	uuid.SetMinCounter()

	fmt.Print("uuid=", uuid, "\n")

	if uuid.Counter() != 0 {
		t.Fatal("uuid must have min counter", uuid)
	}

	key := cdb.NewKey().WithMajorKey("pitOne").WithRegionName(regionName).WithMinorKey("def").WithTimestamp(uuid).Build()
	value := []byte("value")

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, value))

	if err != nil {
		t.Fatal("put failed")
	}

	if !status.Updated() {
		t.Fatal("record must be updated")
	}

	//
	//  Exact Lookup Head
	//

	rec, err := client.Get(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry not found")
	}

	if !key.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	//  Next Key Lookup
	//

	uuidNext := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuidNext.SetUnixTimeMillis(1514764800)
	uuidNext.SetCounter(1)

	keyNext := cdb.NewKey().WithMajorKey("pitOne").WithRegionName(regionName).WithMinorKey("def").WithTimestamp(uuidNext).Build()

	rec, err = client.Get(cdb.NewRequest(keyNext).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if rec.Exist() {
		t.Fatal("entry must not be found, it is the exact lookup")
	}

	//
	//  Next Recent Key Lookup (less case) with same timestamp
	//

	rec, err = client.GetRecent(cdb.NewRequest(keyNext).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must be found")
	}

	if !key.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	//  Recent Key Lookup (equal case) with same timestamp
	//

	rec, err = client.GetRecent(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must be found")
	}

	if !key.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	// Increment milliseconds

	uuidNext = timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuidNext.SetUnixTimeMillis(1514764801)
	uuidNext.SetMinCounter()

	//
	//  Next Recent Key Lookup (less case) with same timestamp
	//

	rec, err = client.GetRecent(cdb.NewRequest(keyNext).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must be found")
	}

	if !key.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	//  Recent Key Lookup (equal case) with same timestamp
	//

	rec, err = client.GetRecent(cdb.NewRequest(key).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must be found")
	}

	if !key.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	//  Test Second Put
	//

	status, err = client.Put(cdb.NewRecord(keyNext, value))

	if err != nil {
		t.Fatal("second put failed")
	}

	if !status.Updated() {
		t.Fatal("entry must be updated")
	}

	//
	//  Search max timestamp
	//

	keyMax := cdb.NewKey().WithMajorKey("pitOne").WithRegionName(regionName).WithMinorKey("def").WithMaxTimestamp().Build()

	rec, err = client.GetRecent(cdb.NewRequest(keyMax).HeadOnly())

	if err != nil {
		t.Fatal("get failed")
	}

	if !rec.Exist() {
		t.Fatal("entry must be found")
	}

	if !keyNext.Timestamp().Equal(rec.Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	//  Search search with different minorKey (would be a different prefix)
	//

	keyMaxWrong := cdb.NewKey().WithMajorKey("pitOne").WithRegionName(regionName).WithMinorKey("deff").WithMaxTimestamp().Build()

	rec, err = client.GetRecent(cdb.NewRequest(keyMaxWrong).HeadOnly())

	if err != nil {
		t.Fatal("get failed", err)
	}

	if rec.Exist() {
		t.Fatal("entry must not found")
	}

	//
	//  Range lookup, records come in DESC order
	//

	block, err := client.GetRange(cdb.NewRangeRequest(keyMax).WithNumRecords(10))

	if err != nil {
		t.Fatal("get range failed", err)
	}

	if len(block) != 2 {
		t.Fatal("expected to be found a two records")
	}

	if !key.Timestamp().Equal(block[1].Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	if !keyNext.Timestamp().Equal(block[0].Key().Timestamp()) {
		t.Fatal("entry must have the same timestamp")
	}

	//
	// Search out of range request
	//

	keyWrong := cdb.NewKey().WithMajorKey("pitTwo").WithRegionName(regionName).WithMinorKey("def").WithMaxTimestamp().Build()

	block, err = client.GetRange(cdb.NewRangeRequest(keyWrong).WithNumRecords(10))

	if err != nil {
		t.Fatal("get range failed", err)
	}

	if len(block) != 0 {
		t.Fatal("expected no records, because of different major key")
	}

}

func RunSpaceTests(t *testing.T, client cdb.Client, regionName string) {

	key := cdb.NewKey().WithMajorKey("alice").WithRegionName(regionName).WithMinorKey("bob")

	hiMessage := []byte("hi")
	hiTimestamp := int64(1514764800)

	// creates TimeUUID with hashed message and timestamp
	status, err := client.Put(cdb.NewRecord(key.WithNamedTimestamp(hiMessage, hiTimestamp).Build(), hiMessage))

	if err != nil {
		t.Fatal("put failed", err)
	}

	if !status.Updated() {
		t.Fatal("entry must be created", err)
	}


	hdudMessage := []byte("how do you do?")
	hdudTimestamp := int64(1514764800)

	status, err = client.Put(cdb.NewRecord(key.WithNamedTimestamp(hdudMessage, hdudTimestamp).Build(), hdudMessage))

	if err != nil {
		t.Fatal("put failed", err)
	}

	if !status.Updated() {
		t.Fatal("entry must be created", err)
	}

	okMessage := []byte("ok")
	okTimestamp := int64(1514764801)

	status, err = client.Put(cdb.NewRecord(key.WithNamedTimestamp(okMessage, okTimestamp).Build(), okMessage))

	if err != nil {
		t.Fatal("put failed", err)
	}

	if !status.Updated() {
		t.Fatal("entry must be created", err)
	}

	chat := make(chan cdb.Block)
	go client.GetRow(cdb.NewRequest(key), chat)

	list := ReadAll(chat)

	if len(list) != 3 {
		t.Fatal("expected 3 messages", err)
	}

	/*
	for _, rec := range list {
		fmt.Print("Record ", string(rec.Value()), "\n")
	}
	*/

	//
	// Send message to eve
	//

	keyEve := cdb.NewKey().WithMajorKey("alice").WithRegionName(regionName).WithMinorKey("eve")

	status, err = client.Put(cdb.NewRecord(keyEve.WithNamedTimestamp(hiMessage, hiTimestamp).Build(), hiMessage))

	if err != nil {
		t.Fatal("put failed", err)
	}

	if !status.Updated() {
		t.Fatal("entry must be created", err)
	}

	//
	// read all messages in alice:CHAT region
	//
	
	chat = make(chan cdb.Block)
	go client.GetRegion(cdb.NewRequest(key), chat)

	list = ReadAll(chat)

	if len(list) != 4 {
		t.Fatal("expected 4 messages", err)
	}

	//
	// read all messages in 'alice' space
	//

	chat = make(chan cdb.Block)
	go client.GetSpace(cdb.NewRequest(key), chat)

	list = ReadAll(chat)

	if len(list) != 4 {
		t.Fatal("expected 4 messages", err)
	}

	//
	// read all records in DB
	//

	chat = make(chan cdb.Block)
	go client.Scan(cdb.NewScanRequest(), chat)

	list = ReadAll(chat)

	/*
	for _, rec := range list {
		fmt.Print("rec=", rec, "\n")
	}
	*/

	if len(list) < 4 {
		t.Fatal("expected 4 or more messages", err)
	}

}


func ReadAll(blockC <-chan cdb.Block) []cdb.Record {

	list := make([]cdb.Record, 0, 100)

	for {
		block, ok := <- blockC

		if !ok {
			break
		}

		for _, rec := range block {
			list = append(list, rec)
		}

	}

	return list

}