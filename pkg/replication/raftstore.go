/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"path/filepath"
	"reflect"

	"go.arpabet.com/glue"
	"go.arpabet.com/store"
	badgerstore "go.arpabet.com/store/providers/badger"
)

/*
RaftStoreFactory provides the "raft-store" managed data store that raftmod's log
and stable store factories consume (they expect a bean named "raft-store" with a
*badger.DB backend). It is a dedicated badger database under
<consensusdb.data-dir>/raft, kept separate from the application key-value data.
*/

var managedDataStoreClass = reflect.TypeOf((*store.ManagedDataStore)(nil)).Elem()

type implRaftStoreFactory struct {
	DataDir string `value:"consensusdb.data-dir,default=/tmp/consensusdb"`
}

func RaftStoreFactory() glue.FactoryBean { return &implRaftStoreFactory{} }

func (t *implRaftStoreFactory) Object() (interface{}, error) {
	dir := filepath.Join(t.DataDir, "raft")
	return badgerstore.New("raft-store", dir)
}

func (t *implRaftStoreFactory) ObjectType() reflect.Type { return managedDataStoreClass }

func (t *implRaftStoreFactory) ObjectName() string { return "raft-store" }

func (t *implRaftStoreFactory) Singleton() bool { return true }
