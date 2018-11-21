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
	"github.com/bigbagger/bigbagger/bbserver"
	"time"
	"github.com/bigbagger/bigbagger/bbclient"
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"os"
	"bytes"
	"fmt"
	"math"
)

const (
	httpAddress=":9481"
	grpcAddress=":9482"
)

func TestSuit(t *testing.T) {

	println("DatasetTest executed")

	dataDir, err := ioutil.TempDir("/tmp", "bigbagger_test")

	if err != nil {
		t.Fatal("fail to create tmp dir ", err)
	}

	defer os.RemoveAll(dataDir)

	conf, err := bbserver.NewDefaultConfiguration(httpAddress, grpcAddress, dataDir)
	if err != nil {
		t.Fatal("fail to create configuration", err)
	}

	server, err := bbserver.NewServer(conf)
	defer server.Close()

	if err != nil {
		t.Fatal("fail to create a bbserver ", err)
		return
	}

	go server.StartServer()

	time.Sleep(time.Second)

	client, err := bbclient.NewClient(grpcAddress)
	defer client.Close()

	if err != nil {
		t.Fatal("fail to create a bbclient ", err)
	}

	table := new(bbproto.Table)
	table.Version = "1.0"
	table.Name = "TEST"
	table.Ttl = "1D"     // one day

	err = client.CreateTable(table)

	if err != nil {
		t.Fatal("fail to TEST dataset ", err)
	}

	table.Name = "TEST_COMPRESS"

	err = client.CreateTable(table)

	if err != nil {
		t.Fatal("fail to create TEST_COMPRESS dataset ", err)
	}

	table.Name = "TEST_ENCRYPT"

	err = client.CreateTable(table)

	if err != nil {
		t.Fatal("fail to create TEST_ENCRYPT dataset ", err)
	}

	table.Name = "TEST_PIT_ONE"
	table.Pit = &bbproto.PointInTime{ PrimaryTimestamp: false, Conflation: false }

	err = client.CreateTable(table)

	list, err := client.DescribeTables("TEST*")

	if err != nil {
		t.Fatal("fail to get dataset ", err)
	}

	if len(list) != 4 {
		t.Fatal("expected 4 results in dataset list, but was: ", len(list))
	}

	m := make(map[string]*bbproto.Table)

	for _, v := range list {
		m[v.Name] = v
	}

	if _, ok := m["TEST"]; !ok {
		t.Fatal("TEST table not found")
	}

	if _, ok := m["TEST_COMPRESS"]; !ok {
		t.Fatal("TEST_COMPRESS table not found")
	}

	if _, ok := m["TEST_ENCRYPT"]; !ok {
		t.Fatal("TEST_ENCRYPT table not found")
	}

	if _, ok := m["TEST_PIT_ONE"]; !ok {
		t.Fatal("TEST_PIT_ONE table not found")
	}

	RunCRUIDTests(t, client, "TEST")
	RunCompareAndSetTests(t, client, "TEST")
	RunWithTtlTests(t, client, "TEST")
	RunCompressionTests(t, client, "TEST_COMPRESS")
	RunEncryptionTests(t, client, "TEST_ENCRYPT")
	RunPitOneTests(t, client, "TEST_PIT_ONE")

	err = client.DropTable("TEST")

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

	op := bbclient.Put(set, []byte("compress"), payload).CompressOnServer()

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


func RunEncryptionTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  One letter with no padding is very good test
	//

	payload := []byte("a")

	//
	//  Test Put
	//

	op := bbclient.Put(set, []byte("enc"), payload).EncryptOnServer()

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}

	//
	//  Test Size
	//

	op = bbclient.Head(set, []byte("enc"))

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

	op = bbclient.Get(set, []byte("enc"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to get entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	if string(res.GetValue()) != "a" {
		t.Fatal("actual value is wrong")
	}

}

func RunPitOneTests(t *testing.T, client bbclient.IBigBagger, set string) {


	//
	//  Test Put
	//

	op := bbclient.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(1514764800)

	res := client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	op = bbclient.Head(set, []byte("pit1")).WithTimestamp(1514764800)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetHead().GetTimestamp(), "\n")

	if res.GetHead().GetTimestamp() != 1514764800 {
		t.Fatal("wrong timestamp")
	}

	//
	//  Lower Lookup Head
	//

	op = bbclient.Head(set, []byte("pit2"))

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

	op = bbclient.Put(set, []byte("pit1"), []byte("value")).WithTimestamp(1514764900)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to put entry ", res.GetError())
	}


	//
	//  Exact Lookup Head
	//

	op = bbclient.Head(set, []byte("pit1")).WithTimestamp(math.MaxUint64)

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if !res.Exists() {
		t.Fatal("entry not found")
	}

	fmt.Print("res.GetHead().GetTimestamp()=", res.GetHead().GetTimestamp(), "\n")

	if res.GetHead().GetTimestamp() != 1514764900 {
		t.Fatal("wrong timestamp")
	}

	//
	//  Lower Lookup Head
	//

	op = bbclient.Head(set, []byte("pit2"))

	res = client.Execute(op)

	if res.IsError() {
		t.Fatal("fail to head entry ", res.GetError())
	}

	if res.Exists() {
		t.Fatal("absent entry found")
	}


}