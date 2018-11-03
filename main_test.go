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

	if err != nil {
		t.Fatal("fail to remove dataset ", err)
	}

	println("remove all files in " + dataDir)
	os.RemoveAll(dataDir)

}

