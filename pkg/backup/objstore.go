/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package backup

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/xerrors"
)

/*
Sinks and sources address a dump by URL:

	/path/to/file            local file (also file:///path)
	s3://bucket/key          S3-compatible object storage

The S3 client speaks the AWS S3 API, so the same code targets AWS S3, MinIO
(open-source, on-prem) and Google Cloud Storage's S3-interoperability endpoint —
switch providers by S3Config.Endpoint alone. When RetainDays>0 the object is
written with object-lock retention (WORM): even a cluster admin cannot alter or
delete it until the retention elapses — the tamper-proof off-site copy the
compliance story needs (the bucket must have object-lock enabled).
*/

// S3Config carries the S3-compatible endpoint credentials and options.
type S3Config struct {
	Endpoint   string // host[:port], empty ⇒ AWS ("s3.amazonaws.com")
	Region     string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	RetainDays int // >0 ⇒ write with object-lock retention (WORM)
}

// OpenSink returns a WriteCloser for the dump destination. Close finalizes the
// write (and, for S3, waits for the upload to complete and reports its error).
func OpenSink(ctx context.Context, dest string, cfg S3Config) (io.WriteCloser, error) {
	bucket, key, ok, err := parseS3(dest)
	if err != nil {
		return nil, err
	}
	if !ok {
		return openFileSink(dest)
	}
	client, err := s3Client(cfg)
	if err != nil {
		return nil, err
	}
	return newS3Sink(ctx, client, bucket, key, cfg.RetainDays), nil
}

// OpenSource returns a ReadCloser for the dump at src.
func OpenSource(ctx context.Context, src string, cfg S3Config) (io.ReadCloser, error) {
	bucket, key, ok, err := parseS3(src)
	if err != nil {
		return nil, err
	}
	if !ok {
		return os.Open(fileURLPath(src))
	}
	client, err := s3Client(cfg)
	if err != nil {
		return nil, err
	}
	obj, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// --- file ----------------------------------------------------------------------

func fileURLPath(dest string) string {
	if strings.HasPrefix(dest, "file://") {
		if u, err := url.Parse(dest); err == nil {
			return u.Path
		}
	}
	return dest
}

func openFileSink(dest string) (io.WriteCloser, error) {
	path := fileURLPath(dest)
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}

// --- s3 ------------------------------------------------------------------------

// parseS3 splits an s3://bucket/key URL; ok=false for non-s3 destinations.
func parseS3(dest string) (bucket, key string, ok bool, err error) {
	if !strings.HasPrefix(dest, "s3://") {
		return "", "", false, nil
	}
	rest := strings.TrimPrefix(dest, "s3://")
	i := strings.IndexByte(rest, '/')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false, xerrors.Errorf("backup: malformed s3 url %q (want s3://bucket/key)", dest)
	}
	return rest[:i], rest[i+1:], true, nil
}

func s3Client(cfg S3Config) (*minio.Client, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	return minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
}

// s3Sink streams writes into a background PutObject via an io.Pipe, so the dump
// uploads as it is produced (no full-size buffering). Close finalizes the upload
// and returns its error.
type s3Sink struct {
	pw   *io.PipeWriter
	done chan error
}

func newS3Sink(ctx context.Context, client *minio.Client, bucket, key string, retainDays int) *s3Sink {
	pr, pw := io.Pipe()
	s := &s3Sink{pw: pw, done: make(chan error, 1)}
	opts := minio.PutObjectOptions{ContentType: "application/octet-stream"}
	if retainDays > 0 {
		opts.Mode = minio.Governance
		until := time.Now().UTC().AddDate(0, 0, retainDays)
		opts.RetainUntilDate = until
	}
	go func() {
		// size unknown ⇒ -1 streams with multipart upload.
		_, err := client.PutObject(ctx, bucket, key, pr, -1, opts)
		// Unblock the writer if the upload failed early.
		_ = pr.CloseWithError(err)
		s.done <- err
	}()
	return s
}

func (s *s3Sink) Write(p []byte) (int, error) { return s.pw.Write(p) }

func (s *s3Sink) Close() error {
	if err := s.pw.Close(); err != nil {
		return err
	}
	return <-s.done // wait for the upload to finish
}
