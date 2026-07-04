/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package backup

import (
	"context"
	"io"

	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valueclient"
	"golang.org/x/xerrors"
)

/*
Client helpers drive the admin.backup / admin.restore value-rpc streams. All
encryption and object-storage handling happens here, on the client: the node
streams raw badger bytes and never sees the password or bucket credentials.
*/

// Backup streams a dump from the node (admin.backup) through the container writer
// (encrypting when password != "") into dest (file or s3://), and returns the max
// badger version — pass it as the next Backup's since for an incremental dump.
func Backup(ctx context.Context, cli valueclient.Client, since uint64, dest, password string, s3 S3Config) (version uint64, err error) {
	sink, err := OpenSink(ctx, dest, s3)
	if err != nil {
		return 0, err
	}
	// Close order matters: finish the container (flush final chunk) before the
	// sink (finish the upload). Errors from either surface.
	container, err := NewWriter(sink, password)
	if err != nil {
		_ = sink.Close()
		return 0, err
	}

	frames, _, err := cli.GetStream(ctx, "admin.backup",
		value.EmptyMap(true).Put("since", value.Long(int64(since))), 8)
	if err != nil {
		_ = container.Close()
		_ = sink.Close()
		return 0, err
	}

	for v := range frames {
		m, ok := v.(value.Map)
		if !ok {
			continue
		}
		if m.GetBool("done").Boolean() {
			version = uint64(m.GetNumber("version").Long())
			continue
		}
		if _, werr := container.Write(m.GetString("data").Raw()); werr != nil {
			err = werr
			break
		}
	}
	if cerr := container.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if serr := sink.Close(); serr != nil && err == nil {
		err = serr
	}
	return version, err
}

// Restore reads a dump from src (file or s3://), decrypts it (password required
// iff the dump is encrypted), and streams the raw bytes to the node
// (admin.restore). The node refuses restore while replication is active.
func Restore(ctx context.Context, cli valueclient.Client, src, password string, s3 S3Config) error {
	source, err := OpenSource(ctx, src, s3)
	if err != nil {
		return err
	}
	defer source.Close()
	reader, err := NewReader(source, password)
	if err != nil {
		return err
	}

	pumpCtx, cancelPump := context.WithCancel(ctx)
	defer cancelPump()

	sendC := make(chan value.Value)
	readErrCh := make(chan error, 1)
	go func() {
		defer close(sendC)
		buf := make([]byte, chunkSize)
		for {
			n, rerr := reader.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case sendC <- value.EmptyMap(true).Put("data", value.Raw(chunk, false)):
				case <-pumpCtx.Done():
					readErrCh <- pumpCtx.Err()
					return
				}
			}
			if rerr == io.EOF {
				readErrCh <- nil
				return
			}
			if rerr != nil {
				readErrCh <- rerr // wrong password / truncated / tampered dump
				return
			}
		}
	}()

	// Chat: stream chunks in, receive the server's single completion frame.
	readC, _, err := cli.Chat(ctx, "admin.restore", value.EmptyMap(true), 4, sendC)
	if err != nil {
		cancelPump()
		return xerrors.Errorf("restore stream: %w", err)
	}
	var serverErr string
	for v := range readC {
		if m, ok := v.(value.Map); ok && m.GetBool("done").Boolean() {
			serverErr = m.GetString("error").String()
		}
	}
	cancelPump()
	readErr := <-readErrCh

	// The client-side read error (decrypt/truncation) is the true cause; the
	// server error (a load failure) is secondary.
	if readErr != nil && !xerrors.Is(readErr, context.Canceled) {
		return xerrors.Errorf("read dump: %w", readErr)
	}
	if serverErr != "" {
		return xerrors.Errorf("restore: %s", serverErr)
	}
	return nil
}
