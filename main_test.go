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
)

const (
	grpcAddress=":9482"
)

func TestSuit(t *testing.T) {

	println("DatasetTest executed")

	dataDir, err := ioutil.TempDir("/tmp", "bigbagger_test")

	if err != nil {
		t.Fatal("fail to create tmp dir ", err)
		return
	}

	defer os.RemoveAll(dataDir)

	server, err := bbserver.NewServer(dataDir)
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

	dataset.Name = "TEST_SECOND"
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

	if _, ok := m["TEST_SECOND"]; !ok {
		t.Fatal("TEST_SECOND dataset not found")
	}

	err = client.DeleteDataset("TEST")

	RunCRUIDTests(t, client, "TEST_SECOND")
	RunCompareAndSetTests(t, client, "TEST_SECOND")
	RunWithTtlTests(t, client, "TEST_SECOND")

	if err != nil {
		t.Fatal("fail to remove dataset ", err)
	}


}

func RunCRUIDTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("key"))

	res, err := client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists", err)
	}

	//
	//  Test Put
	//

	op = bbclient.Put(set, []byte("key"), []byte("value"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o put entry ", err)
	}

	if res.IsError() {
		t.Fatal("remove fail to put entry ", res.GetError())
	}

	//
	//  Test Exists
	//

	op = bbclient.Head(set, []byte("key"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if !res.Exists() {
		t.Fatal("entry must exists", err)
	}

	//
	//  Test Get
	//

	op = bbclient.Get(set, []byte("key"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o get entry ", err)
	}

	if res.IsError() {
		t.Fatal("remove fail to get entry ", res.GetError())
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

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o remove entry ", err)
	}

	if res.IsError() {
		t.Fatal("remove fail to remove entry ", res.GetError())
	}

	//
	//  Test Not Exists
	//

	op = bbclient.Head(set, []byte("key"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if res.Exists() {
		t.Fatal("entry nust be removed", err)
	}


}


func RunCompareAndSetTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("cas"))

	res, err := client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists", err)
	}

	//
	//  Test Put If Absent
	//

	op = bbclient.Put(set, []byte("cas"), []byte("first"))
	op.CompareAndSet(0)

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o putIfAbsent entry ", err)
	}

	if !res.Updated() {
		t.Fatal("for empty entries version is always 0", err)
	}

	//
	//  Test Get First
	//

	op = bbclient.Get(set, []byte("cas"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o get entry ", err)
	}

	if res.GetValue() == nil {
		t.Fatal("entry not found", err)
	}

	if string(res.GetValue()) != "first" {
		t.Fatal("wrong value of the first entry", err)
	}

	firstVersion := res.GetVersion()

	if firstVersion <= 0 {
		t.Fatal("wrong value of the first version", err)
	}

	//fmt.Print("firstVersion=", firstVersion, "\n")

	//
	//  Test Replace
	//

	op = bbclient.Put(set, []byte("cas"), []byte("second"))
	op.CompareAndSet(firstVersion)

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o replace entry ", err)
	}

	if !res.Updated() {
		t.Fatal("compareAndSet not triggered", err)
	}

	//
	//  Test Get Second
	//

	op = bbclient.Get(set, []byte("cas"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o get entry ", err)
	}

	if res.GetValue() == nil {
		t.Fatal("entry not found", err)
	}

	if string(res.GetValue()) != "second" {
		t.Fatal("wrong value of the second entry", err)
	}

	secondVersion := res.GetVersion()

	if secondVersion <= firstVersion {
		t.Fatal("wrong value of the second version", err)
	}

	//fmt.Print("secondVersion=", secondVersion, "\n")

	//
	//  Test Remove
	//

	op = bbclient.Remove(set, []byte("cas"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o remove entry ", err)
	}

	if res.IsError() {
		t.Fatal("remove fail to remove entry ", res.GetError())
	}


}


func RunWithTtlTests(t *testing.T, client bbclient.IBigBagger, set string) {

	//
	//  Test Not Exists
	//

	op := bbclient.Head(set, []byte("ttl"))

	res, err := client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if res.Exists() {
		t.Fatal("this is a new test, entry must not exists", err)
	}

	if res.GetExpiresAt() > 0 {
		t.Fatal("expected zero for expiration time", err)
	}

	//
	//  Test Put With TTL
	//

	op = bbclient.Put(set, []byte("ttl"), []byte("value")).WithTtl(100)

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o put entry ", err)
	}

	if res.IsError() {
		t.Fatal("remove fail to put entry ", res.GetError())
	}

	//
	//  Test Exists With TTL
	//

	op = bbclient.Head(set, []byte("ttl"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o exists entry ", err)
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found", err)
	}

	//fmt.Print("ExpireAt=", res.GetExpiresAt(), "\n")

	if res.GetExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time", err)
	}

	//
	//  Test Get With TTL
	//

	op = bbclient.Get(set, []byte("ttl"))

	res, err = client.Execute(op)

	if err != nil {
		t.Fatal("i/o get entry ", err)
	}

	if !res.Exists() {
		t.Fatal("value with ttl not found", err)
	}

	if res.GetExpiresAt() == 0 {
		t.Fatal("expected non zero for expiration time", err)
	}

	if string(res.GetValue()) != "value" {
		t.Fatal("wrong value with ttl", err)
	}

}