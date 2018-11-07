/*
 *
 * Copyright 2018-present Alexander Shvid and other authors
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
	"bigbagger/bbserver"
	"time"
	"bigbagger/bbclient"
	"bigbagger/proto/bbproto"
	"os"
	"bytes"
)

const (
	grpcAddress=":9482"
)

func TestSuit(t *testing.T) {

	println("DatasetTest executed")

	dataDir, err := ioutil.TempDir("/tmp", "bigbagger_test")

	if err != nil {
		t.Fatal("fail to create tmp dir ", err)
	}

	defer os.RemoveAll(dataDir)

	security, err := bbserver.NewSimpleSecurityContext("TEST")
	if err != nil {
		t.Fatal("fail to create security", err)
	}

	server, err := bbserver.NewServer(dataDir, security)
	defer server.Close()

	if err != nil {
		t.Fatal("fail to create a bbserver ", err)
		return
	}

	go server.StartServer(grpcAddress)

	time.Sleep(time.Second)

	client, err := bbclient.NewClient(grpcAddress)
	defer client.Close()

	if err != nil {
		t.Fatal("fail to create a bbclient ", err)
	}

	dataset := new(bbproto.Dataset)
	dataset.Version = "1.0"
	dataset.Name = "TEST"

	err = client.CreateDataset(dataset)

	if err != nil {
		t.Fatal("fail to create dataset ", err)
	}

	dataset.Name = "TEST_COMPRESS"
	dataset.Compression = new(bbproto.Compression)
	dataset.Compression.Compressor = bbproto.Compressor_COMPRESS_FLATE
	dataset.Compression.Level = bbproto.CompressionLevel_BEST_COMPRESSION
	dataset.Compression.Threshold = 100  // do not compress payloads less than 100 bytes

	err = client.CreateDataset(dataset)

	if err != nil {
		t.Fatal("fail to create second dataset ", err)
	}

	list, err := client.GetDataset("*")

	if err != nil {
		t.Fatal("fail to get dataset ", err)
	}

	if len(list) != 2 {
		t.Fatal("expected 2 results in dataset list, but was: ", len(list))
	}

	m := make(map[string]*bbproto.Dataset)

	for _, v := range list {
		m[v.Name] = v
	}

	if _, ok := m["TEST"]; !ok {
		t.Fatal("TEST dataset not found")
	}

	if _, ok := m["TEST_COMPRESS"]; !ok {
		t.Fatal("TEST_COMPRESS dataset not found")
	}

	RunCRUIDTests(t, client, "TEST")
	RunCompareAndSetTests(t, client, "TEST")
	RunWithTtlTests(t, client, "TEST")
	RunCompressionTests(t, client, "TEST_COMPRESS")

	err = client.DeleteDataset("TEST")

	if err != nil {
		t.Fatal("fail to remove dataset ", err)
	}


}

func RunCRUIDTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("key"))

	res := client.Execute(op)

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists")
	}

	//
	//  Test Put
	//

	op = bbclient.Put(set, []byte("key"), []byte("value"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Exists
	//

	op = bbclient.Head(set, []byte("key"))

	res = client.Execute(op)

	if !res.Exists() {
		t.Fatal("entry must exists")
	}

	//
	//  Test Get
	//

	op = bbclient.Get(set, []byte("key"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	data := res.GetValue()

	if data == nil {
		t.Fatal("entry not found")
	}

	if string(data) != "value" {
		t.Fatal("wrong data found")
	}

	//
	//  Test Remove
	//

	op = bbclient.Remove(set, []byte("key"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to remove entry ", res.GetError())
	}

	//
	//  Test Not Exists
	//

	op = bbclient.Head(set, []byte("key"))

	res = client.Execute(op)

	if res.Exists() {
		t.Fatal("entry nust be removed")
	}


}


func RunCompareAndSetTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("cas"))

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

	op = bbclient.Put(set, []byte("cas"), []byte("first"))
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

	op = bbclient.Get(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if res.GetValue() == nil {
		t.Fatal("entry not found")
	}

	if string(res.GetValue()) != "first" {
		t.Fatal("wrong value of the first entry")
	}

	firstVersion := res.GetHead().GetVersion()

	if firstVersion <= 0 {
		t.Fatal("wrong value of the first version")
	}

	//fmt.Print("firstVersion=", firstVersion, "\n")

	//
	//  Test Replace
	//

	op = bbclient.Put(set, []byte("cas"), []byte("second"))
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

	op = bbclient.Get(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if res.GetValue() == nil {
		t.Fatal("entry not found")
	}

	if string(res.GetValue()) != "second" {
		t.Fatal("wrong value of the second entry")
	}

	secondVersion := res.GetHead().GetVersion()

	if secondVersion <= firstVersion {
		t.Fatal("wrong value of the second version")
	}

	//fmt.Print("secondVersion=", secondVersion, "\n")

	//
	//  Test Remove
	//

	op = bbclient.Remove(set, []byte("cas"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to remove entry ", res.GetError())
	}


}


func RunWithTtlTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("ttl"))

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get head", res.GetError())
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists")
	}

	if res.GetHead().GetExpiresAt() > 0 {
		t.Fatal("expected zero for expiration time")
	}

	//
	//  Test Put With TTL
	//

	op = bbclient.Put(set, []byte("ttl"), []byte("value")).WithTtl(100)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Exists With TTL
	//

	op = bbclient.Head(set, []byte("ttl"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get head", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found")
	}

	//fmt.Print("ExpireAt=", res.GetExpiresAt(), "\n")

	if res.GetHead().GetExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time")
	}

	//
	//  Test Get With TTL
	//

	op = bbclient.Get(set, []byte("ttl"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found")
	}

	if res.GetHead().GetExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time")
	}

	if string(res.GetValue()) != "value" {
		t.Fatal("wrong value with ttl")
	}

	firstExpiresAt := res.GetHead().GetExpiresAt()

	//
	//  Test Touch
	//

	op = bbclient.Touch(set, []byte("ttl")).WithTtl(1000)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to touch", res.GetError())
	}

	if !res.Updated() {
		t.Fatal("touch did not update result")
	}

	if firstExpiresAt >= res.GetHead().GetExpiresAt() {
		t.Fatal("after touch expire at time must be changed")
	}

}


func RunCompressionTests(t *testing.T, client bbclient.IBigBagger, set string) {

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

	op := bbclient.Put(set, []byte("compress"), payload)

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Size
	//

	op = bbclient.Head(set, []byte("compress"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if res.GetHead().GetDiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	//
	//  Test Get
	//

	op = bbclient.Get(set, []byte("compress"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if res.GetHead().GetDiskSize() > 1000 {
		t.Fatal("value must be compressed")
	}

	if !bytes.Equal(payload, res.GetValue()) {
		t.Fatal("actual value is not the same as payload")
	}

}
