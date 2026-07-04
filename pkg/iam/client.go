/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package iam

import (
	"context"

	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valueclient"
)

/*
Thin client helpers for the iam CLI: they speak the same kv.* value-rpc wire
convention as the data plane (and the store/providers/cdb client), addressed at
the system tenant. Used by `consensusdb iam …` against a running node.
*/

// keyValue encodes an IAM record key on the wire.
func keyValue(minor string) value.Value {
	return value.EmptyMap(true).
		Put("major", value.Raw([]byte(SystemTenant), false)).
		Put("region", value.Raw([]byte(Region), false)).
		Put("minor", value.Raw([]byte(minor), false))
}

// CreateRecord writes an IAM record create-if-absent (CAS version 0). It returns
// false when the record already exists.
func CreateRecord(ctx context.Context, cli valueclient.Client, minor string, val []byte) (bool, error) {
	return putRecord(ctx, cli, minor, val, true)
}

// PutRecord writes an IAM record unconditionally (used for index records).
func PutRecord(ctx context.Context, cli valueclient.Client, minor string, val []byte) error {
	_, err := putRecord(ctx, cli, minor, val, false)
	return err
}

// GetRecord reads and decodes an IAM record; found=false when absent.
func GetRecord(ctx context.Context, cli valueclient.Client, minor string, obj interface{}) (found bool, err error) {
	req := value.EmptyMap(true).Put("key", keyValue(minor)).Put("head_only", value.Boolean(false))
	res, err := cli.CallFunction(ctx, "kv.get", req)
	if err != nil {
		return false, err
	}
	m, ok := res.(value.Map)
	if !ok || !m.GetBool("found").Boolean() {
		return false, nil
	}
	if err := Decode(m.GetString("value").Raw(), obj); err != nil {
		return false, err
	}
	return true, nil
}

func putRecord(ctx context.Context, cli valueclient.Client, minor string, val []byte, createOnly bool) (bool, error) {
	req := value.EmptyMap(true).
		Put("key", keyValue(minor)).
		Put("value", value.Raw(val, false)).
		Put("metadata", value.Long(0)).
		Put("ttl", value.Long(0)).
		Put("cas", value.Boolean(createOnly)).
		Put("version", value.Long(0)). // with cas: 0 = create-if-absent
		Put("expires_at", value.Long(0))
	res, err := cli.CallFunction(ctx, "kv.put", req)
	if err != nil {
		return false, err
	}
	if m, ok := res.(value.Map); ok {
		return m.GetBool("updated").Boolean(), nil
	}
	return false, nil
}
