package bbserver

import (
	"github.com/dgraph-io/badger"
	"bigbagger/proto/bbproto"
	"os"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/jsonpb"
	"bigbagger/bbcommon"
	"fmt"
)

const (
	DATASET_JSON = "dataset.json"
)

type DatasetContext struct {
	db *badger.DB
	dataset *bbproto.Dataset
}

func (this *DatasetContext) GetName() string {
	return this.dataset.Name
}

func (this *DatasetContext) Close() error {
	if this.db != nil {
		println("Close dataset: " + this.dataset.Name)
		return this.db.Close()
	}
	return nil
}

func NewDataset(dbDir string, dataset *bbproto.Dataset) (context *DatasetContext, err error) {

	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err = os.Mkdir(dbDir, 0755)
		if err != nil {
			return nil, err
		}

		str, err := new(jsonpb.Marshaler).MarshalToString(dataset)
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteFile(filepath.Join(dbDir, DATASET_JSON), []byte(str), 0755)

		if err != nil {
			return nil, err
		}

	}

	return OpenDataset(dbDir, dataset)

}

func OpenDataset(dbDir string, dataset *bbproto.Dataset) (context *DatasetContext, err error) {

	context = new(DatasetContext)
	context.dataset = dataset

	opts := badger.DefaultOptions
	opts.Dir = dbDir + "/key"
	opts.ValueDir = dbDir + "/value"
	context.db, err = badger.Open(opts)

	return context, err

}

func LoadDataset(dbDir string) (context *DatasetContext, err error) {

	data, err := ioutil.ReadFile(filepath.Join(dbDir, DATASET_JSON))

	if err != nil {
		return nil, err
	}

	dataset := new(bbproto.Dataset)
	err = jsonpb.UnmarshalString(string(data), dataset)

	if err != nil {
		return nil, err
	}

	return OpenDataset(dbDir, dataset)

}

func (this *DatasetContext) ProcessExistsOperation(key *bbproto.Key, operation *bbproto.ExistsOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)
	if err != nil {
		return bbcommon.SuccessExistsResult(0)
	}

	return bbcommon.SuccessExistsResult(item.Version())

}

func (this *DatasetContext) ProcessGetOperation(key *bbproto.Key, operation *bbproto.GetOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get failed: ", err))
	}

	data, err := item.Value()
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get fetch failed: ", err))
	}

	return bbcommon.SuccessGetResult(data, item.Version())

}

func (this *DatasetContext) ProcessTouchOperation(key *bbproto.Key, operation *bbproto.TouchOperation) *bbproto.RecordResult {

	return nil

}

func (this *DatasetContext) ProcessPutOperation(key *bbproto.Key, operation *bbproto.PutOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)

	err := txn.Set(key.RecordKey, operation.Value)

	if err != nil {
		txn.Discard()
		return bbcommon.ErrorDriver(fmt.Sprint("set failed: ", err))
	}

	err = txn.Commit(nil)

	return bbcommon.SuccessPutResult()

}

func (this *DatasetContext) ProcessRemoveOperation(key *bbproto.Key, operation *bbproto.RemoveOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)

	err := txn.Delete(key.RecordKey)

	if err != nil {
		txn.Discard()
		return bbcommon.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	err = txn.Commit(nil)

	return bbcommon.SuccessRemoveResult()

}


func (this *DatasetContext) ProcessOperation(operation *bbproto.RecordOperation) *bbproto.RecordResult {

	switch operation.Operation.(type) {

		case *bbproto.RecordOperation_Exists:
			return this.ProcessExistsOperation(operation.Key, operation.GetExists())

		case *bbproto.RecordOperation_Get:
			return this.ProcessGetOperation(operation.Key, operation.GetGet())

		case *bbproto.RecordOperation_Touch:
			return this.ProcessTouchOperation(operation.Key, operation.GetTouch())

		case *bbproto.RecordOperation_Put:
			return this.ProcessPutOperation(operation.Key, operation.GetPut())

		case *bbproto.RecordOperation_Remove:
			return this.ProcessRemoveOperation(operation.Key, operation.GetRemove())

	}

	return bbcommon.ErrorUnsupported("unknown operation type")
}
