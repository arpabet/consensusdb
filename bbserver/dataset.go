package bbserver

import (
	"github.com/dgraph-io/badger"
	"bigbagger/proto/bbproto"
)

type DatasetContext struct {
	db *badger.DB
	proto *bbproto.Dataset
}

func (this *DatasetContext) Close() error {
	if this.db != nil {
		return this.db.Close()
	}
	return nil
}

func NewDataset(dataDir string, proto *bbproto.Dataset) (context *DatasetContext, err error) {

	context = new(DatasetContext)
	context.proto = proto

	dir := dataDir + "/" + proto.Name

	opts := badger.DefaultOptions
	opts.Dir = dir + "/key"
	opts.ValueDir = dir + "/value"
	context.db, err = badger.Open(opts)

	return context, err

}


