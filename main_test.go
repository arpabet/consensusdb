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
	"go.uber.org/zap"
)


func TestSuit(t *testing.T) {

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

	log := zap.NewExample()

	server, err := cserver.NewServer(conf, log)
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
	RunCompressionTests(t, client, regionName)
	RunEncryptionTests(t, client, regionName)
	//RunPitOneTests(t, client, regionName)

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

	if bytes.Equal(firstValue, rec.Value()) {
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

	if bytes.Equal(secondValue, rec.Value()) {
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


func RunCompressionTests(t *testing.T, client cdb.Client, regionName string) {

	key := cdb.NewKey().WithMajorKey("compression").WithRegionName(regionName).WithMinorKey("def").Build()

	//
	//  Create Payload
	//

	payload := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		payload[i] = byte(i)
	}

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, payload).UseCompression(cdb.LZ4_HIGH))

	if err != nil {
		t.Fatal("fail to put entry")
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


func RunEncryptionTests(t *testing.T, client cdb.Client, regionName string) {

	key := cdb.NewKey().WithMajorKey("encryption").WithRegionName(regionName).WithMinorKey("def").Build()

	//
	//  One letter with no padding is a very good test
	//

	payload := []byte("a")

	//
	//  Test Put
	//

	status, err := client.Put(cdb.NewRecord(key, payload).UseEncryption(cdb.AES, cdb.CFB))

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
		t.Fatal("get failed")
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

	if bytes.Equal(payload, rec.Value()) {
		t.Fatal("actual value is wrong")
	}

}

/*
func RunPitOneTests(t *testing.T, client cdb.Client, regionName string) {

	uuid := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuid.SetUnixTimeMillis(1514764800)
	uuid.SetCounter(rand.Int63())

	//
	//  Test Put
	//

	op := cdb.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(uuid)

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	op = cdb.Get(set, []byte("pit1")).HeadOnly().WithTimestamp(uuid)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetRecord().Head().Timestamp(), "\n")

	if !res.GetRecord().Head().Timestamp().Equal(uuid) {
		t.Fatal("wrong timestamp")
	}

	//
	//  Lower Lookup Head
	//

	op = cdb.Get(set, []byte("pit2")).HeadOnly()

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if res.Exists() {
		t.Fatal("absent entry found")
	}

	//
	//  Test Second Put
	//

	op = cdb.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(uuid)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	uuidMax := timeuuid.NewUUID(timeuuid.TimebasedVer1)
	uuidMax.SetTime100NanosUnsigned(math.MaxUint64)
	uuidMax.SetMaxCounter()

	op = cdb.GetEarly(set, []byte("pit1"), 1).HeadOnly().WithTimestamp(uuidMax)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetRecord().Head().Timestamp(), "\n")

	if !res.GetRecord().Head().Timestamp().Equal(uuid) {
		t.Fatal("wrong timestamp")
	}

	//
	//  Lower Lookup Head
	//

	op = cdb.GetEarly(set, []byte("pit2"), 1).HeadOnly()

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if res.Exists() {
		t.Fatal("absent entry found")
	}


}

*/