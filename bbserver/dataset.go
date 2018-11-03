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

func (this *DatasetContext) ProcessHeadOperation(key *bbproto.Key, operation *bbproto.HeadOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)
	if err != nil {
		return SuccessHeadNotFoundResult()
	}

	return SuccessHeadResult(key.Timestamp, item)

}

func (this *DatasetContext) ProcessGetOperation(key *bbproto.Key, operation *bbproto.GetOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key.RecordKey)
	if err != nil {
		return SuccessGetNotFoundResult()
	}

	data, err := item.Value()
	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("get failed: ", err))
	}

	return SuccessGetResult(key.Timestamp, data, item)

}

func (this *DatasetContext) ProcessTouchOperation(key *bbproto.Key, operation *bbproto.TouchOperation) *bbproto.RecordResult {

	return SuccessTouchResult()

}

func (this *DatasetContext) ProcessPutOperation(key *bbproto.Key, operation *bbproto.PutOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	if operation.CompareAndSet {

		item, err := txn.Get(key.RecordKey)

		if err != nil {
			if operation.Version != 0 {
				return SuccessPutNotUpdatedResult()
			}
		} else if operation.Version != item.Version() {
			return SuccessPutNotUpdatedResult()
		}

	}

	err := txn.Set(key.RecordKey, operation.Value)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put failed: ", err))
	}

	err = txn.Commit(nil)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("put commit failed: ", err))
	}

	return SuccessPutResult()

}

func (this *DatasetContext) ProcessRemoveOperation(key *bbproto.Key, operation *bbproto.RemoveOperation) *bbproto.RecordResult {

	txn := this.db.NewTransaction(true)
    defer txn.Discard()

	err := txn.Delete(key.RecordKey)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove failed: ", err))
	}

	err = txn.Commit(nil)

	if err != nil {
		return bbcommon.ErrorDriver(fmt.Sprint("remove commit failed: ", err))
	}

	return SuccessRemoveResult()

}


func (this *DatasetContext) ProcessOperation(operation *bbproto.RecordOperation) *bbproto.RecordResult {

	switch operation.Operation.(type) {

		case *bbproto.RecordOperation_Head:
			return this.ProcessHeadOperation(operation.Key, operation.GetHead())

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
