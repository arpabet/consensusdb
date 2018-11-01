package bbserver

import (
	"github.com/dgraph-io/badger"
	"bigbagger/proto/bbproto"
	"os"
	"io/ioutil"
	"path/filepath"
	"github.com/golang/protobuf/jsonpb"
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
