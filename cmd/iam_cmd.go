/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valueclient"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
IamGroup roots the identity management commands (`iam bootstrap|user-add|sa-add`)
that write identity records into the system tenant over the node's value-rpc data
plane (iam.address, default this node).

The enablement flow follows etcd: deploy with auth.enabled=false, create the
identities with these commands, then set auth.enabled=true and restart. Once auth
is enabled, set iam.user/iam.password (or iam.token) so the CLI itself can
authenticate — e.g. IAM_USER / IAM_PASSWORD in the environment.
*/
type IamGroup struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (IamGroup) Group() string { return "iam" }

func (IamGroup) Help() (string, string) {
	return "identity management (users, service accounts)", ""
}

// iamDial connects to the data plane, attaching the CLI's own credential when
// configured (needed once auth.enabled=true).
type iamDial struct {
	Address  string `value:"iam.address,default=tcp://127.0.0.1:8444"`
	User     string `value:"iam.user,default="`
	Password string `value:"iam.password,default="`
	Token    string `value:"iam.token,default="`
}

func (t *iamDial) run(cb func(ctx context.Context, cli valueclient.Client) error) error {
	// glue does not inject value: tags on embedded struct fields (it treats an
	// anonymous field as interface exposure), so these values arrive empty. Resolve
	// them from the environment and defaults here, which also honors the documented
	// IAM_ADDRESS / IAM_TOKEN / IAM_USER / IAM_PASSWORD overrides.
	address := firstNonEmpty(t.Address, os.Getenv("IAM_ADDRESS"), "tcp://127.0.0.1:8444")
	token := firstNonEmpty(t.Token, os.Getenv("IAM_TOKEN"))
	user := firstNonEmpty(t.User, os.Getenv("IAM_USER"))
	password := firstNonEmpty(t.Password, os.Getenv("IAM_PASSWORD"))

	cli := valueclient.NewClient(address, "")
	switch {
	case token != "":
		cli.SetCredential(value.EmptyMap(true).
			Put("method", value.Utf8("token")).Put("token", value.Utf8(token)))
	case user != "":
		cli.SetCredential(value.EmptyMap(true).
			Put("method", value.Utf8("password")).
			Put("user", value.Utf8(user)).Put("pass", value.Utf8(password)))
	}
	if err := cli.Connect(); err != nil {
		return xerrors.Errorf("connect %s: %v", address, err)
	}
	defer cli.Close()
	return cb(context.Background(), cli)
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// generatePassword returns a random password when none was supplied.
func generatePassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// IamBootstrapCommand creates the initial admin user (idempotent: reports when
// the user already exists instead of overwriting).
type IamBootstrapCommand struct {
	Parent   cligo.CliGroup `cli:"group=iam"`
	Username string         `cli:"argument=username,required"`
	Password string         `cli:"option=password,default=,help=admin password (generated and printed when empty)"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamBootstrapCommand) Command() string { return "bootstrap" }

func (t *IamBootstrapCommand) Help() (string, string) {
	return "create the initial admin user",
		"Creates the initial admin user in the system tenant. Run once while auth.enabled=false, then enable authentication and restart the nodes."
}

func (t *IamBootstrapCommand) Run(ctx context.Context) error {
	password := t.Password
	generated := false
	if password == "" {
		var err error
		if password, err = generatePassword(); err != nil {
			return err
		}
		generated = true
	}
	hash, err := iam.HashPassword(password)
	if err != nil {
		return err
	}
	record, err := iam.Encode(&iam.UserRecord{
		Name: t.Username, PasswordHash: hash, Admin: true, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		created, err := iam.CreateRecord(ctx, cli, iam.UserPrefix+t.Username, record)
		if err != nil {
			return err
		}
		if !created {
			fmt.Printf("user %q already exists — bootstrap already done\n", t.Username)
			return nil
		}
		fmt.Printf("admin user %q created\n", t.Username)
		if generated {
			fmt.Printf("password (shown once): %s\n", password)
		}
		fmt.Println("next: set auth.enabled=true (AUTH_ENABLED=true) and restart the nodes")
		return nil
	})
}

// IamUserAddCommand creates a password-authenticated user.
type IamUserAddCommand struct {
	Parent   cligo.CliGroup `cli:"group=iam"`
	Username string         `cli:"argument=username,required"`
	Password string         `cli:"option=password,default=,help=password (generated and printed when empty)"`
	Admin    bool           `cli:"option=admin,default=false,help=mark as admin"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamUserAddCommand) Command() string { return "user-add" }

func (t *IamUserAddCommand) Help() (string, string) {
	return "create a user (password login)", ""
}

func (t *IamUserAddCommand) Run(ctx context.Context) error {
	password := t.Password
	generated := false
	if password == "" {
		var err error
		if password, err = generatePassword(); err != nil {
			return err
		}
		generated = true
	}
	hash, err := iam.HashPassword(password)
	if err != nil {
		return err
	}
	record, err := iam.Encode(&iam.UserRecord{
		Name: t.Username, PasswordHash: hash, Admin: t.Admin, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		created, err := iam.CreateRecord(ctx, cli, iam.UserPrefix+t.Username, record)
		if err != nil {
			return err
		}
		if !created {
			return xerrors.Errorf("user %q already exists", t.Username)
		}
		fmt.Printf("user %q created\n", t.Username)
		if generated {
			fmt.Printf("password (shown once): %s\n", password)
		}
		return nil
	})
}

// IamSaAddCommand creates a service account with an API token and optional
// client-certificate identities (SAN URI or CN) for mTLS login.
type IamSaAddCommand struct {
	Parent     cligo.CliGroup `cli:"group=iam"`
	Name       string         `cli:"argument=name,required"`
	CertIdents string         `cli:"option=cert-idents,default=,help=comma-separated certificate identities (SAN URI or CN) that authenticate as this account"`
	iamDial
	Log *zap.Logger `inject:""`
}

func (t *IamSaAddCommand) Command() string { return "sa-add" }

func (t *IamSaAddCommand) Help() (string, string) {
	return "create a service account (token and/or mTLS login)", ""
}

func (t *IamSaAddCommand) Run(ctx context.Context) error {
	token, tokenHash, err := iam.GenerateToken(t.Name)
	if err != nil {
		return err
	}
	var idents []string
	for _, ident := range strings.Split(t.CertIdents, ",") {
		if ident = strings.TrimSpace(ident); ident != "" {
			idents = append(idents, ident)
		}
	}
	record, err := iam.Encode(&iam.ServiceAccountRecord{
		Name: t.Name, TokenHash: tokenHash, CertIdentities: idents, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return t.run(func(ctx context.Context, cli valueclient.Client) error {
		created, err := iam.CreateRecord(ctx, cli, iam.ServiceAccountPrefix+t.Name, record)
		if err != nil {
			return err
		}
		if !created {
			return xerrors.Errorf("service account %q already exists", t.Name)
		}
		for _, ident := range idents {
			idx, err := iam.Encode(&iam.CertIndexRecord{ServiceAccount: t.Name})
			if err != nil {
				return err
			}
			if err := iam.PutRecord(ctx, cli, iam.CertPrefix+ident, idx); err != nil {
				return xerrors.Errorf("cert identity %q: %w", ident, err)
			}
		}
		fmt.Printf("service account %q created\n", t.Name)
		fmt.Printf("token (shown once): %s\n", token)
		if len(idents) > 0 {
			fmt.Printf("certificate identities: %s\n", strings.Join(idents, ", "))
		}
		return nil
	})
}
