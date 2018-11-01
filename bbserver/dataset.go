package bbserver

import (
	"github.com/dgraph-io/badger"
	"bigbagger/proto/bbproto"
	"os"
	"encoding/json"
	"go.etcd.io/etcd/pkg/ioutil"
)

type DatasetContext struct {
	db *badger.DB
	dataset *bbproto.Dataset
}

func (this *DatasetContext) Close() error {
	if this.db != nil {
		println("Close dataset: " + this.dataset.Name)
		return this.db.Close()
	}
	return nil
}

func NewDataset(dataDir string, dataset *bbproto.Dataset) (context *DatasetContext, err error) {

	context = new(DatasetContext)
	context.dataset = dataset

	dbDir := dataDir + "/" + dataset.Name

	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err = os.Mkdir(dbDir, 0755)
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(dataset)
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteAndSyncFile(dbDir + "/dataset.json", data, 0755)
		if err != nil {
			return nil, err
		}

	}

	opts := badger.DefaultOptions
	opts.Dir = dbDir + "/key"
	opts.ValueDir = dbDir + "/value"
	context.db, err = badger.Open(opts)

	return context, err

}


