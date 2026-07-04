/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"fmt"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/backup"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
Backup / restore CLI (plan S4). Encryption and object-storage credentials live
here on the client — the node only streams raw bytes. Destinations/sources are a
local path or an s3://bucket/key URL (AWS S3, MinIO, or GCS via the S3 endpoint).

	consensusdb backup  s3://backups/cdb/full.dump --password "$PW"
	consensusdb restore s3://backups/cdb/full.dump --password "$PW"

S3 credentials come from properties/env: BACKUP_S3_ENDPOINT, BACKUP_S3_REGION,
BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY, BACKUP_S3_USE_SSL, BACKUP_S3_RETAIN_DAYS.
The dump password comes from --password or BACKUP_PASSWORD (empty ⇒ plain dump).
*/

// backupDial connects to the admin control surface, attaching a credential when
// auth is enabled (same convention as the iam CLI).
type backupDial struct {
	Address  string `value:"admin.address,default=tcp://127.0.0.1:8444"`
	User     string `value:"iam.user,default="`
	Password string `value:"iam.password,default="`
	Token    string `value:"iam.token,default="`
}

func (t *backupDial) run(cb func(ctx context.Context, cli valueclient.Client) error) error {
	cli := valueclient.NewClient(t.Address, "")
	switch {
	case t.Token != "":
		cli.SetCredential(value.EmptyMap(true).
			Put("method", value.Utf8("token")).Put("token", value.Utf8(t.Token)))
	case t.User != "":
		cli.SetCredential(value.EmptyMap(true).
			Put("method", value.Utf8("password")).
			Put("user", value.Utf8(t.User)).Put("pass", value.Utf8(t.Password)))
	}
	if err := cli.Connect(); err != nil {
		return xerrors.Errorf("connect %s: %v", t.Address, err)
	}
	defer cli.Close()
	return cb(context.Background(), cli)
}

// s3Options holds the object-storage config injected from properties/env.
type s3Options struct {
	Endpoint   string `value:"backup.s3.endpoint,default="`
	Region     string `value:"backup.s3.region,default="`
	AccessKey  string `value:"backup.s3.access-key,default="`
	SecretKey  string `value:"backup.s3.secret-key,default="`
	UseSSL     bool   `value:"backup.s3.use-ssl,default=true"`
	RetainDays int    `value:"backup.s3.retain-days,default=0"`
	Password   string `value:"backup.password,default="`
}

func (o s3Options) config() backup.S3Config {
	return backup.S3Config{
		Endpoint:   o.Endpoint,
		Region:     o.Region,
		AccessKey:  o.AccessKey,
		SecretKey:  o.SecretKey,
		UseSSL:     o.UseSSL,
		RetainDays: o.RetainDays,
	}
}

// resolvePassword prefers the explicit --password flag, else the injected one.
func (o s3Options) resolvePassword(flag string) string {
	if flag != "" {
		return flag
	}
	return o.Password
}

// BackupCommand streams a dump from the cluster to dest (file or s3://).
type BackupCommand struct {
	Parent   cligo.CliGroup `cli:"group=cli"`
	Dest     string         `cli:"argument=dest,required"`
	Since    int            `cli:"option=since,default=0,help=incremental backup: only entries after this version (0 = full)"`
	Password string         `cli:"option=password,default=,help=encrypt with this password (empty = plain dump; or BACKUP_PASSWORD)"`
	backupDial
	s3Options
	Log *zap.Logger `inject:""`
}

func (t *BackupCommand) Command() string { return "backup" }

func (t *BackupCommand) Help() (string, string) {
	return "back up the store to a file or s3://bucket/key",
		"Streams a dump from the cluster; encrypt with --password (argon2id + AES-256-GCM). Destinations: local path or s3://bucket/key (AWS S3, MinIO, GCS)."
}

func (t *BackupCommand) Run(ctx context.Context) error {
	password := t.s3Options.resolvePassword(t.Password)
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		version, err := backup.Backup(ctx, cli, uint64(t.Since), t.Dest, password, t.config())
		if err != nil {
			return err
		}
		encrypted := "plain"
		if password != "" {
			encrypted = "encrypted"
		}
		fmt.Printf("backup complete (%s) → %s\n", encrypted, t.Dest)
		fmt.Printf("max version: %d  (pass --since %d for the next incremental)\n", version, version)
		return nil
	})
}

// RestoreCommand loads a dump from src (file or s3://) into a fresh node.
type RestoreCommand struct {
	Parent   cligo.CliGroup `cli:"group=cli"`
	Src      string         `cli:"argument=src,required"`
	Password string         `cli:"option=password,default=,help=password for an encrypted dump (or BACKUP_PASSWORD)"`
	backupDial
	s3Options
	Log *zap.Logger `inject:""`
}

func (t *RestoreCommand) Command() string { return "restore" }

func (t *RestoreCommand) Help() (string, string) {
	return "restore the store from a file or s3://bucket/key",
		"Loads a dump into a FRESH node (refused while replication is active — restore, then bootstrap the cluster)."
}

func (t *RestoreCommand) Run(ctx context.Context) error {
	password := t.s3Options.resolvePassword(t.Password)
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		if err := backup.Restore(ctx, cli, t.Src, password, t.config()); err != nil {
			return err
		}
		fmt.Printf("restore complete ← %s\n", t.Src)
		return nil
	})
}
