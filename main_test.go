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
	"fmt"
	"math"
	"log"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
)

const (
	httpAddress=":9481"
	grpcAddress=":9482"
)

func TestSuit(t *testing.T) {

	println("DatasetTest executed")

	rootDir, err := ioutil.TempDir("/tmp", "consensusdb_test")

	if err != nil {
		t.Fatal("fail to create tmp dir ", err)
	}

	defer os.RemoveAll(rootDir)

	conf, err := cserver.NewDefaultConfiguration(httpAddress, grpcAddress, rootDir)
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
	go server.RaftLoop()

	time.Sleep(time.Second)

	client, err := cdb.NewClient(grpcAddress)
	defer client.Close()

	if err != nil {
		t.Fatal("fail to create a cdb ", err)
	}

	RunCRUIDTests(t, client, "TEST")
	RunCompareAndSetTests(t, client, "TEST")
	RunWithTtlTests(t, client, "TEST")
	RunCompressionTests(t, client, "TEST")
	RunEncryptionTests(t, client, "TEST")
	RunPitOneTests(t, client, "TEST_PIT")

	list, err := client.GetRegions("TEST*")

	if err != nil {
		t.Fatal("fail to get region list ", err)
	}

	if len(list) != 2 {
		t.Fatal("expected 2 results in dataset list, but was: ", len(list))
	}

	m := make(map[string]*cserverpb.Region)

	for _, v := range list {
		m[v.Name] = v
	}

	if _, ok := m["TEST"]; !ok {
		t.Fatal("TEST table not found")
	}

	if _, ok := m["TEST_PIT"]; !ok {
		t.Fatal("TEST_PIT table not found")
	}

	err = client.DeleteRegion("TEST")

	if err != nil {
		t.Fatal("fail to remove dataset ", err)
	}


}

func RunCRUIDTests(t *testing.T, client cdb.IConsensusDB, set string) {

	//
	//  Test Not Exists
	//

	op := cdb.Get(set, []byte("key")).HeadOnly()

	res := client.Execute(op)

	log.Println("res=", res)

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists")
	}

	//
	//  Test Put
	//

	op = cdb.Put(set, []byte("key"), []byte("value"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Exists
	//

	op = cdb.Get(set, []byte("key")).HeadOnly()

	res = client.Execute(op)

	if !res.Exists() {
		t.Fatal("entry must exists")
	}

	//
	//  Test Get
	//

	op = cdb.Get(set, []byte("key"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	data := res.GetRecord().Value()

	if data == nil {
		t.Fatal("entry not found")
	}

	if string(data) != "value" {
		t.Fatal("wrong data found")
	}

	//
	//  Test Remove
	//

	op = cdb.Remove(set, []byte("key"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to remove entry ", res.GetError())
	}

	//
	//  Test Not Exists
	//

	op = cdb.Get(set, []byte("key")).HeadOnly()

	res = client.Execute(op)

	if res.Exists() {
		t.Fatal("entry nust be removed")
	}


}


func RunCompareAndSetTests(t *testing.T, client cdb.IConsensusDB, set string) {

	//
	//  Test Not Exists
	//

	op := cdb.Get(set, []byte("cas")).HeadOnly()

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get head", res.GetError())
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists")
	}

	//
	//  Test Put If Absent
	//

	op = cdb.Put(set, []byte("cas"), []byte("first"))
	op.CompareAndSet(0)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to compare and set", res.GetError())
	}

	if !res.Updated() {
		t.Fatal("put if absent failed")
	}

	//
	//  Test Get First
	//

	op = cdb.Get(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if res.GetRecord().Value() == nil {
		t.Fatal("entry not found")
	}

	if string(res.GetRecord().Value()) != "first" {
		t.Fatal("wrong value of the first entry")
	}

	firstVersion := res.GetRecord().Head().Version()

	if firstVersion <= 0 {
		t.Fatal("wrong value of the first version")
	}

	//fmt.Print("firstVersion=", firstVersion, "\n")

	//
	//  Test Replace
	//

	op = cdb.Put(set, []byte("cas"), []byte("second"))
	op.CompareAndSet(firstVersion)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to update", res.GetError())
	}

	if !res.Updated() {
		t.Fatal("compareAndSet not triggered")
	}

	//
	//  Test Get Second
	//

	op = cdb.Get(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if string(res.GetRecord().Value()) != "second" {
		t.Fatal("wrong value of the second entry")
	}

	secondVersion := res.GetRecord().Head().Version()

	if secondVersion <= firstVersion {
		t.Fatal("wrong value of the second version")
	}

	//fmt.Print("secondVersion=", secondVersion, "\n")

	//
	//  Test Remove
	//

	op = cdb.Remove(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to remove entry ", res.GetError())
	}


}


func RunWithTtlTests(t *testing.T, client cdb.IConsensusDB, set string) {

	//
	//  Test Not Exists
	//

	op := cdb.Get(set, []byte("ttl")).HeadOnly()

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get head", res.GetError())
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists")
	}

	if res.GetRecord().Head().ExpiresAt() > 0 {
		t.Fatal("expected zero for expiration time")
	}

	//
	//  Test Put With TTL
	//

	op = cdb.Put(set, []byte("ttl"), []byte("value")).WithTtl(100)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Exists With TTL
	//

	op = cdb.Get(set, []byte("ttl")).HeadOnly()

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get head", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found")
	}

	//fmt.Print("ExpireAt=", res.GetExpiresAt(), "\n")

	if res.GetRecord().Head().ExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time")
	}

	//
	//  Test Get With TTL
	//

	op = cdb.Get(set, []byte("ttl"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found")
	}

	if res.GetRecord().Head().ExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time")
	}

	if string(res.GetRecord().Value()) != "value" {
		t.Fatal("wrong value with ttl")
	}

	firstExpiresAt := res.GetRecord().Head().ExpiresAt()

	//
	//  Test Touch
	//

	op = cdb.Touch(set, []byte("ttl")).WithTtl(1000)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to touch", res.GetError())
	}

	if !res.Updated() {
		t.Fatal("touch did not update result")
	}

	if firstExpiresAt >= res.GetRecord().Head().ExpiresAt() {
		t.Fatal("after touch expire at time must be changed")
	}

}


func RunCompressionTests(t *testing.T, client cdb.IConsensusDB, set string) {

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

	op := cdb.Put(set, []byte("compress"), payload).CompressOnServer()

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Size
	//

	op = cdb.Get(set, []byte("compress")).HeadOnly()

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if res.GetRecord().Head().DiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	//
	//  Test Get
	//

	op = cdb.Get(set, []byte("compress"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if res.GetRecord().Head().DiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	if !bytes.Equal(payload, res.GetRecord().Value()) {
		t.Fatal("actual value is not the same as payload")
	}

}


func RunEncryptionTests(t *testing.T, client cdb.IConsensusDB, set string) {

	//
	//  One letter with no padding is very good test
	//

	payload := []byte("a")

	//
	//  Test Put
	//

	op := cdb.Put(set, []byte("enc"), payload).EncryptOnServer()

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Size
	//

	op = cdb.Get(set, []byte("enc")).HeadOnly()

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	//
	//  Test Get
	//

	op = cdb.Get(set, []byte("enc"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if string(res.GetRecord().Value()) != "a" {
		t.Fatal("actual value is wrong")
	}

}

func RunPitOneTests(t *testing.T, client cdb.IConsensusDB, set string) {


	//
	//  Test Put
	//

	op := cdb.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(1514764800)

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	op = cdb.Get(set, []byte("pit1")).HeadOnly().WithTimestamp(1514764800)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetRecord().Head().Timestamp(), "\n")

	if res.GetRecord().Head().Timestamp() != 1514764800 {
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

	op = cdb.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(1514764900)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	op = cdb.GetEarly(set, []byte("pit1"), 1).HeadOnly().WithTimestamp(math.MaxUint64)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetRecord().Head().Timestamp(), "\n")

	if res.GetRecord().Head().Timestamp() != 1514764900 {
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