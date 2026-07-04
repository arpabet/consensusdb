/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package verify checks that a backup dump corresponds to a quorum-agreed
checkpoint. It loads the dump into a throwaway store, reads the persisted
hash-chain head, and confirms a quorum certificate attests exactly that head —
so an auditor can prove, entirely offline, that a backup is the state a majority
of the cluster certified at a given height. It reports progress so a CLI or a
background job can drive a progress bar.
*/
package verify

import (
	"context"
	"io"
	"os"
	"time"

	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

func nowUnix() int64 { return time.Now().Unix() }

// openSized opens the backup source with its byte size (0 = unknown) for progress.
func openSized(ctx context.Context, opts Options) (io.ReadCloser, int64, error) {
	return backup.OpenSourceSized(ctx, opts.Source, opts.S3)
}

// Options configure a backup verification. The trust material (CA cert, quorum
// certificate, node certs) is supplied so verification needs no running cluster.
type Options struct {
	Source     string          // backup path or s3://bucket/key
	Password   string          // dump password ("" for a plain dump)
	S3         backup.S3Config // S3 credentials for an s3:// source
	CACert     []byte          // ledger CA public key
	QuorumCert []byte          // encoded QuorumCertificate
	NodeCerts  [][]byte        // encoded node certs for the signers
	Threshold  int             // minimum signers (0 ⇒ the certificate's own signer count)
}

// Result is the outcome of a verification.
type Result struct {
	Verified bool   `json:"verified"`
	Height   uint64 `json:"height"`
	Digest   string `json:"digest"`  // hex chain digest attested and found
	Signers  int    `json:"signers"` // signers in the quorum certificate
	Message  string `json:"message"`
}

// VerifyBackup runs the verification, reporting progress in [0,100]. It returns a
// Result (Verified false with a Message on a verification mismatch) or an error
// for operational failures (unreadable source, bad trust material).
func VerifyBackup(ctx context.Context, opts Options, progress func(pct int)) (*Result, error) {
	report := func(p int) {
		if progress != nil {
			progress(p)
		}
	}
	report(0)

	// 1. Parse the trust material and verify the quorum certificate up front.
	ca, err := ledger.ParseCAPublicKey(opts.CACert)
	if err != nil {
		return nil, xerrors.Errorf("verify: CA public key: %w", err)
	}
	qc, err := ledger.DecodeQuorum(opts.QuorumCert)
	if err != nil {
		return nil, xerrors.Errorf("verify: quorum certificate: %w", err)
	}
	bundle := ledger.CertBundle{}
	for _, raw := range opts.NodeCerts {
		cert, err := ledger.DecodeCert(raw)
		if err != nil {
			return nil, xerrors.Errorf("verify: node cert: %w", err)
		}
		bundle.AddCert(cert)
	}
	threshold := opts.Threshold
	if threshold == 0 {
		threshold = len(qc.Signers)
	}
	if err := ledger.Verify(ca, qc, bundle, threshold, nowUnix()); err != nil {
		return &Result{Verified: false, Message: "quorum certificate invalid: " + err.Error()}, nil
	}
	report(5)

	// 2. Load the dump into a throwaway store, tracking progress by bytes read.
	head, err := loadBackupHead(ctx, opts, func(pct int) { report(5 + pct*90/100) })
	if err != nil {
		return nil, err
	}
	report(95)

	// 3. The quorum certificate must attest exactly this head.
	if err := qc.MatchesHead(head.index, head.digest); err != nil {
		return &Result{
			Verified: false, Height: qc.Checkpoint.Height,
			Digest:  ledger.DigestHex(head.digest),
			Signers: len(qc.Signers),
			Message: "backup does not match the certified checkpoint: " + err.Error(),
		}, nil
	}
	report(100)
	return &Result{
		Verified: true, Height: head.index, Digest: ledger.DigestHex(head.digest),
		Signers: len(qc.Signers),
		Message: "backup matches the quorum-certified checkpoint",
	}, nil
}

type chainHead struct {
	index  uint64
	digest [32]byte
}

// loadBackupHead opens the backup source, decrypts + loads it into a temp store,
// and returns the persisted chain head.
func loadBackupHead(ctx context.Context, opts Options, progress func(pct int)) (chainHead, error) {
	dir, err := os.MkdirTemp("", "cdb-verify-*")
	if err != nil {
		return chainHead{}, err
	}
	defer os.RemoveAll(dir)

	conf := &server.Configuration{DataDir: dir, FileIO: true}
	if err := conf.PostConstruct(); err != nil {
		return chainHead{}, err
	}
	storage, err := server.OpenKeyValueStorage(conf, zap.NewNop())
	if err != nil {
		return chainHead{}, err
	}
	defer storage.Close()

	source, size, err := openSized(ctx, opts)
	if err != nil {
		return chainHead{}, err
	}
	defer source.Close()

	counting := &countingReader{r: source, total: size, cb: progress}
	reader, err := backup.NewReader(counting, opts.Password)
	if err != nil {
		return chainHead{}, err
	}
	if err := storage.Load(reader); err != nil {
		return chainHead{}, xerrors.Errorf("verify: load dump: %w", err)
	}

	index, digest := ledger.LoadHead(storage)
	return chainHead{index: index, digest: digest}, nil
}

// countingReader reports progress by fraction of the source consumed.
type countingReader struct {
	r     io.Reader
	total int64
	read  int64
	last  int
	cb    func(pct int)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	if c.total > 0 && c.cb != nil {
		if pct := int(c.read * 100 / c.total); pct != c.last {
			c.last = pct
			c.cb(pct)
		}
	}
	return n, err
}
