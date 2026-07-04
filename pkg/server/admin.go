/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"
	"io"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

/*
AdminService exposes cluster administration over the value-rpc control surface
(plan S4): streaming backup and restore of the whole store. It runs on the same
vrpc server as the data plane and is authorized by the cdb.backups.* permissions
(instance scope) — encryption and object-storage credentials stay entirely on the
client (the `consensusdb backup` CLI), so a node never holds a backup password or
bucket key.

	admin.backup   server → client: streams the raw badger backup bytes as frames,
	               then a final frame carrying the max version (for incremental
	               backups). Read-only, safe on any node.
	admin.restore  client → server: loads a raw badger dump into local storage.
	               This bypasses raft, so it is refused while replication is active
	               (a live cluster would diverge); restore into a fresh node, then
	               bootstrap. See the README runbook.
*/
type AdminService struct {
	Server     valueserver.Server `inject:""`
	Storage    KeyValueStorage    `inject:""`
	Replicator Replicator         `inject:"optional"`
	Policy     *PolicyService     `inject:"optional"`
	Log        *zap.Logger        `inject:""`
}

func (t *AdminService) BeanName() string { return "admin-service" }

func (t *AdminService) PostConstruct() error {
	if err := valueserver.AddOutgoingStreamTyped(t.Server, "admin.backup",
		backupRequestCodec, backupFrameCodec, t.backup); err != nil {
		t.Log.Error("AdminRegisterBackup", zap.Error(err))
	}
	// Restore is a chat (bidi): the client streams dump chunks in and receives a
	// single completion frame back, so it learns when the load actually finished
	// (and whether it failed) rather than fire-and-forget.
	if err := t.Server.AddChat("admin.restore", valuerpc.Any, t.restore); err != nil {
		t.Log.Error("AdminRegisterRestore", zap.Error(err))
	}
	return nil
}

// backupFrame is one item of the backup stream: a data chunk, or the terminal
// frame with the max version and done=true.
type backupFrame struct {
	Data    []byte
	Version uint64
	Done    bool
}

var backupRequestCodec = valuerpc.Codec[uint64]{
	Encode: func(since uint64) value.Value { return value.EmptyMap(true).Put("since", value.Long(int64(since))) },
	Decode: func(v value.Value) (uint64, error) {
		if m, ok := v.(value.Map); ok {
			return uint64(m.GetNumber("since").Long()), nil
		}
		return 0, nil
	},
}

var backupFrameCodec = valuerpc.Codec[*backupFrame]{
	Encode: func(f *backupFrame) value.Value {
		m := value.EmptyMap(true).Put("done", value.Boolean(f.Done))
		if f.Done {
			return m.Put("version", value.Long(int64(f.Version)))
		}
		return m.Put("data", value.Raw(f.Data, false))
	},
	Decode: func(v value.Value) (*backupFrame, error) {
		m, ok := v.(value.Map)
		if !ok {
			return nil, valuerpc.NewError(valuerpc.CodeInternal, "malformed backup frame")
		}
		if m.GetBool("done").Boolean() {
			return &backupFrame{Done: true, Version: uint64(m.GetNumber("version").Long())}, nil
		}
		return &backupFrame{Data: m.GetString("data").Raw()}, nil
	},
}

// backupChunk bounds each stream frame; the badger backup is copied through in
// pieces so the whole dump is never buffered.
const backupChunk = 64 * 1024

func (t *AdminService) backup(ctx context.Context, since uint64) (<-chan *backupFrame, error) {
	if err := t.Policy.Authorize(ctx, iam.PermBackupsCreate, "", ""); err != nil {
		return nil, err
	}
	out := make(chan *backupFrame)
	pr, pw := io.Pipe()
	verCh := make(chan uint64, 1)

	go func() {
		v, err := t.Storage.Backup(pw, since)
		_ = pw.CloseWithError(err)
		verCh <- v // published after the pipe closes; read only after EOF below
	}()

	go func() {
		defer close(out)
		buf := make([]byte, backupChunk)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case out <- &backupFrame{Data: chunk}:
				case <-ctx.Done():
					return
				}
			}
			if err == io.EOF {
				select {
				case out <- &backupFrame{Done: true, Version: <-verCh}:
				case <-ctx.Done():
				}
				return
			}
			if err != nil {
				t.Log.Error("AdminBackup", zap.Error(err))
				return
			}
		}
	}()
	return out, nil
}

// restore is a chat handler: it must return quickly, draining inC and loading in
// a goroutine, and reports completion (or failure) on the returned channel.
func (t *AdminService) restore(ctx context.Context, _ value.Value, inC <-chan value.Value) (<-chan value.Value, error) {
	if err := t.Policy.Authorize(ctx, iam.PermBackupsRestore, "", ""); err != nil {
		return nil, err
	}
	// Restore bypasses the raft log; on a live replicated cluster it would
	// diverge the nodes. Only allow it when replication is inactive.
	if t.Replicator != nil && t.Replicator.Enabled() {
		return nil, valuerpc.NewError(valuerpc.CodeInvalidArgument,
			"restore is disabled while replication is active: restore into a fresh node, then bootstrap")
	}

	out := make(chan value.Value, 1)
	pr, pw := io.Pipe()
	loadDone := make(chan error, 1)
	go func() { loadDone <- t.Storage.Load(pr) }()

	go func() {
		defer close(out)
		var writeErr error
		for v := range inC {
			if m, ok := v.(value.Map); ok {
				if _, err := pw.Write(m.GetString("data").Raw()); err != nil {
					writeErr = err
					break
				}
			}
		}
		_ = pw.CloseWithError(writeErr)
		loadErr := <-loadDone
		if loadErr == nil {
			loadErr = writeErr
		}

		frame := value.EmptyMap(true).Put("done", value.Boolean(true))
		if loadErr != nil {
			t.Log.Error("AdminRestore", zap.Error(loadErr))
			frame = frame.Put("error", value.Utf8(loadErr.Error()))
		} else {
			t.Log.Info("AdminRestore", zap.String("status", "loaded"))
		}
		out <- frame
	}()
	return out, nil
}
