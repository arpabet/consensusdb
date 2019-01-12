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

package cserver

import (
	stats "go.etcd.io/etcd/etcdserver/api/v2stats"
	"go.etcd.io/etcd/etcdserver/api/rafthttp"
	"go.etcd.io/etcd/etcdserver/api/snap"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/consensusdb/consensusdb/c"
	"net/http"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"strconv"
	"go.uber.org/zap"
	"go.etcd.io/etcd/pkg/types"
	"go.etcd.io/etcd/wal"
	"go.etcd.io/etcd/wal/walpb"
	"time"
)

type DefaultServer struct {

	grpcServer       *grpc.Server
	conf             *Configuration
	kv               IStorage

	shuttingDown     bool

	logger           *zap.Logger

	wal              *wal.WAL
	snapshotter      *snap.Snapshotter

	ticker 			*time.Ticker

	raftStorage      *raft.MemoryStorage
	raftNode         raft.Node
	raftTransport    *rafthttp.Transport

	raftErrorC 		 chan error
	raftStopC  		 chan bool
}

func (this *DefaultServer) Close() {

	if this == nil || this.shuttingDown {
		return
	}

	this.shuttingDown = true

	log.Println("consensusdb shutting down...")

	this.raftStopC <- true
	this.ticker.Stop()

	if this.grpcServer != nil {
		this.grpcServer.Stop()
		this.grpcServer = nil
	}

	this.kv.Close()

	close(this.raftErrorC)

	this.wal.Close()
}

func NewServer(conf *Configuration) (server *DefaultServer, err error) {

	log.Printf("Data path: %s\n", conf.DataDir)

	kv, err := OpenDefaultStore(conf);
	if err != nil {
		return nil, err
	}

	server = &DefaultServer{
		conf:           conf,
		kv:             kv,
		logger:         zap.NewExample(),
		ticker:         time.NewTicker(100 * time.Millisecond),
		raftStorage:    raft.NewMemoryStorage(),
		raftErrorC:     make(chan error),
		raftStopC:      make(chan bool, 1),
	}

	server.snapshotter = snap.New(server.logger, conf.SnapDir)

	walExist := wal.Exist(conf.WalDir)

	if !walExist {

		w, err := wal.Create(server.logger, conf.WalDir, nil)
		if err != nil {
			return nil, err
		}
		w.Close()

	}

	snapshot, err := server.snapshotter.Load()
	walSnapshot := walpb.Snapshot{}
	if err != nil {
		if err != snap.ErrNoSnapshot {
			log.Fatal("error loading raft snapshot", err)
		}
	} else {
		walSnapshot.Index, walSnapshot.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
		server.raftStorage.ApplySnapshot(*snapshot)
	}

	log.Printf("Loading WAL at term %d and index %d", walSnapshot.Term, walSnapshot.Index)
	server.wal, err = wal.Open(server.logger, conf.WalDir, walSnapshot)
	if err != nil {
		return nil, err
	}

	_, st, ents, err := server.wal.ReadAll()
	if err != nil {
		log.Fatal("failed to read WAL", err)
	}
	server.raftStorage.SetHardState(st)
	server.raftStorage.Append(ents)

	if len(ents) > 0 {
		lastIndex := ents[len(ents)-1].Index
		log.Print("raft started from lastIndex=", lastIndex, "\n")
	}

	raftConf := &raft.Config{
		ID:                        uint64(conf.PeerId),
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   server.raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	raftPeers := make([]raft.Peer, 0, len(conf.Peers))
	for k, _ := range conf.Peers {
		raftPeers = append(raftPeers, raft.Peer{ID: uint64(k)})
	}

	if walExist {
		server.raftNode = raft.RestartNode(raftConf)
	} else {
		server.raftNode = raft.StartNode(raftConf, raftPeers)
	}

	peerIdStr := strconv.Itoa(conf.PeerId)

	server.raftTransport = &rafthttp.Transport{
		Logger:      zap.NewExample(),
		ID:          types.ID(conf.PeerId),
		ClusterID:   types.ID(conf.ClusterId),
		Raft:        server,
		ServerStats: stats.NewServerStats(conf.PeerName, peerIdStr),
		LeaderStats: stats.NewLeaderStats(peerIdStr),
		ErrorC:      server.raftErrorC,
	}

	err = server.raftTransport.Start()
	if err != nil {
		return nil, err
	}

	for k, v := range conf.Peers {
		if k != conf.PeerId {
			server.raftTransport.AddPeer(types.ID(k), []string{v})
		}
	}

	return server,nil
}


func (this *DefaultServer) GetRaftMux()  *http.ServeMux {
	handler := this.raftTransport.Handler()
	return handler.(*http.ServeMux)
}

func (this *DefaultServer) saveToStorage() {

}

func (this *DefaultServer) RaftLoop() error {

	// event loop on raft state machine updates
	for {
		select {
		case <-this.ticker.C:
			this.raftNode.Tick()

			// store raft entries to wal, then publish over commit channel
		case rd := <-this.raftNode.Ready():
			this.wal.Save(rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				log.Print("Received snapshot")
				this.saveSnapshot(rd.Snapshot)
				this.raftStorage.ApplySnapshot(rd.Snapshot)
				//this.publishSnapshot(rd.Snapshot)
			}

			this.raftStorage.Append(rd.Entries)
			this.raftTransport.Send(rd.Messages)


			this.raftNode.Advance()

		case err := <-this.raftTransport.ErrorC:
			log.Fatal("raft transport error", err)
			return err

		case <-this.raftStopC:
			return nil
		}
	}
}

func (this *DefaultServer) saveSnapshot(snap raftpb.Snapshot) error {
	// must save the snapshot index to the WAL before saving the
	// snapshot to maintain the invariant that we only Open the
	// wal at previously-saved snapshot indexes.
	walSnapshot := walpb.Snapshot{
		Index: snap.Metadata.Index,
		Term:  snap.Metadata.Term,
	}
	if err := this.wal.SaveSnapshot(walSnapshot); err != nil {
		return err
	}
	if err := this.snapshotter.SaveSnap(snap); err != nil {
		return err
	}
	return this.wal.ReleaseLockTo(snap.Metadata.Index)
}

//
//  Raft
//

func (this *DefaultServer) Process(ctx context.Context, m raftpb.Message) error {
	return this.raftNode.Step(ctx, m)
}

func (this *DefaultServer) IsIDRemoved(id uint64) bool                           {
	return false
}

func (this *DefaultServer) ReportUnreachable(id uint64)                          {

}

func (this *DefaultServer) ReportSnapshot(id uint64, status raft.SnapshotStatus) {

}

func (this *DefaultServer) propogateChanges(msg *cserverpb.RawRecord) {
}

func (this *DefaultServer) getSnapshot() ([]byte, error) {

	majorKey := []byte{}

	var outC chan *cserverpb.RawRecord

	this.kv.GetSnapshot(majorKey, outC)

	return []byte{}, nil
}


//
//
// Database API
//
//

func (this *DefaultServer) Execute(context context.Context, tx *cserverpb.Transaction) (response *cserverpb.TransactionResult, err error) {

	size := len(tx.Operations)

	response = new(cserverpb.TransactionResult)
	response.Results = make([]*cserverpb.TxOperationResult, 0, size)

	if size == 0 {
		return response, nil
	}

	tnx := this.kv.NewTransaction()

	for i := 0; i < size; i = i + 1 {
		if c.IsUpdateOperation(tx.Operations[i]) {
			tnx.SetUpdate(true)
		}
	}

	tnx.Begin()

	rollbackAll := false

	for i := 0; i < size; i = i + 1 {
		result := tnx.ProcessOperation(tx.Operations[i])
		response.Results = append(response.Results, result)

		if !c.IsSuccessCode(result.Status) {
			rollbackAll = true
			break
		}

	}

	if rollbackAll {
		tnx.Rollback()
	} else {
		tnx.Commit()
	}

	return response, nil

}

func (this *DefaultServer) ServeGRPC() error {

	// start listening for grpc
	listen, err := net.Listen("tcp4", this.conf.GrpcAddress)
	if err != nil {
		log.Fatal("port is busy " + this.conf.GrpcAddress, err)
		return err
	}

	// Create new grpc cserver
	this.grpcServer = grpc.NewServer()

	// Register services
	cserverpb.RegisterKeyValueServiceServer(this.grpcServer, this)

	// Start serving requests
	return this.grpcServer.Serve(listen)

}
