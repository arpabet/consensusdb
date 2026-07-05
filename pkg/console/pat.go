/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/pb"
)

/*
Personal access tokens (PATs) for users: expiring bearer tokens ("pat-…") a user
authenticates with, minted and revoked by an admin from the Users tab. Each PAT is
a token/<hash> → TokenRecord{principal:user:<name>, expiresAt, label, createdAt} in
the same reverse index as service-account tokens, so authentication (hash lookup +
expiry check) already handles them. The token itself is shown once.
*/

const defaultPATDays = 90

type patOut struct {
	ID        string `json:"id"` // the token hash — the revoke handle, not the token
	Label     string `json:"label"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

// iamListUserTokens lists a user's PATs by scanning the token index for records
// whose principal is this user.
func (t *ConsoleHandler) iamListUserTokens(w http.ResponseWriter, user string) {
	principal := iam.PrincipalUser(user)
	out := []patOut{}
	err := t.scanIAM(func(minor string, value []byte) {
		hash, ok := strings.CutPrefix(minor, iam.TokenIndexPrefix)
		if !ok {
			return
		}
		rec := &iam.TokenRecord{}
		if iam.Decode(value, rec) == nil && rec.Principal == principal {
			out = append(out, patOut{ID: hash, Label: rec.Label, CreatedAt: rec.CreatedAt, ExpiresAt: rec.ExpiresAt})
		}
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "scan tokens")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

// iamCreateUserToken mints an expiring PAT for a user. The token is returned once.
func (t *ConsoleHandler) iamCreateUserToken(w http.ResponseWriter, r *http.Request, user string) {
	if !t.principalExists(iam.PrincipalUser(user)) {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	var req struct {
		Label   string `json:"label"`
		TTLDays int    `json:"ttlDays"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	days := req.TTLDays
	if days <= 0 {
		days = defaultPATDays
	}
	token, hash, err := iam.NewToken(iam.TokenPrefixUser)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	rec := &iam.TokenRecord{
		Principal: iam.PrincipalUser(user),
		ExpiresAt: now.AddDate(0, 0, days).Unix(),
		Label:     strings.TrimSpace(req.Label),
		CreatedAt: now.Unix(),
	}
	raw, err := iam.Encode(rec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode token")
		return
	}
	if _, err := t.svc.Put(context.Background(), &pb.RecordRequest{Key: iam.Key(iam.TokenIndexKey(hash)), Value: raw}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": hash, "token": token, "label": rec.Label, "expiresAt": rec.ExpiresAt,
		"note": "the token is shown once — copy it now",
	})
}

// iamRevokeUserToken deletes a user's PAT, verifying it belongs to that user first.
func (t *ConsoleHandler) iamRevokeUserToken(w http.ResponseWriter, user, id string) {
	if id == "" {
		writeErr(w, http.StatusBadRequest, "token id required")
		return
	}
	rec, err := t.svc.Get(context.Background(), &pb.KeyRequest{Key: iam.Key(iam.TokenIndexKey(id))})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil || len(rec.Value) == 0 {
		writeErr(w, http.StatusNotFound, "token not found")
		return
	}
	tr := &iam.TokenRecord{}
	if iam.Decode(rec.Value, tr) != nil || tr.Principal != iam.PrincipalUser(user) {
		writeErr(w, http.StatusNotFound, "token not found for this user")
		return
	}
	if _, err := t.svc.Remove(context.Background(), &pb.KeyRequest{Key: iam.Key(iam.TokenIndexKey(id))}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": id})
}
