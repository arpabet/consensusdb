/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"time"

	"go.arpabet.com/consensusdb/pkg/iam"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/server"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

/*
LedgerService exposes the verifiable ledger over the value-rpc surface (plan S6):
`ledger.digest` returns this node's current hash-chain checkpoint together with
its BLS signature and CA-issued cert. A client collects the digest from a quorum
of nodes (all converge to the same head at a committed height) and aggregates the
signatures into a QuorumCertificate — a compact, offline-verifiable proof that a
majority agreed on exactly this history.

Signing is opt-in: set ledger.node-key and ledger.node-cert to this node's
identity files (see the `ledger keygen` / `ledger issue` CLI). Without them the
digest is still served, unsigned, so the chain head is always readable.
*/
type LedgerService struct {
	Server  valueserver.Server    `inject:""`
	FSM     *FSM                  `inject:""`
	Policy  *server.PolicyService `inject:"optional"`
	Log     *zap.Logger           `inject:""`
	KeyPath string                `value:"ledger.node-key,default="`
	CrtPath string                `value:"ledger.node-cert,default="`

	signer *ledger.NodeSigner
}

func (t *LedgerService) BeanName() string { return "ledger-service" }

func (t *LedgerService) PostConstruct() error {
	if t.KeyPath != "" && t.CrtPath != "" {
		s, err := ledger.LoadNodeSigner(t.KeyPath, t.CrtPath)
		if err != nil {
			t.Log.Error("LedgerSignerLoad", zap.Error(err))
		} else {
			t.signer = s
			t.Log.Info("LedgerSignerLoaded", zap.String("nodeId", s.NodeID()))
		}
	}
	if err := valueserver.AddUnary(t.Server, "ledger.digest",
		anyValueCodec, digestCodec, t.digest); err != nil {
		t.Log.Error("LedgerRegisterDigest", zap.Error(err))
	}
	return nil
}

// anyValueCodec passes a value.Value through unchanged — for handlers that take
// no meaningful request (ledger.digest).
var anyValueCodec = valuerpc.Codec[value.Value]{
	Encode: func(v value.Value) value.Value {
		if v == nil {
			return value.EmptyMap(true)
		}
		return v
	},
	Decode: func(v value.Value) (value.Value, error) { return v, nil },
}

// digestResponse is one node's signed statement of its chain head.
type digestResponse struct {
	checkpoint *ledger.Checkpoint
	nodeID     string
	signature  []byte
	cert       []byte // encoded NodeCert (empty when unsigned)
}

var digestCodec = valuerpc.Codec[*digestResponse]{
	Encode: func(r *digestResponse) value.Value {
		m := value.EmptyMap(true).
			Put("height", value.Long(int64(r.checkpoint.Height))).
			Put("term", value.Long(int64(r.checkpoint.Term))).
			Put("digest", value.Raw(r.checkpoint.Digest, false)).
			Put("unix", value.Long(r.checkpoint.Unix)).
			Put("node_id", value.Utf8(r.nodeID)).
			Put("signature", value.Raw(r.signature, false)).
			Put("cert", value.Raw(r.cert, false))
		return m
	},
	Decode: func(v value.Value) (*digestResponse, error) {
		m, ok := v.(value.Map)
		if !ok {
			return nil, valuerpc.NewError(valuerpc.CodeInternal, "malformed digest")
		}
		return &digestResponse{
			checkpoint: &ledger.Checkpoint{
				Height: uint64(m.GetNumber("height").Long()),
				Term:   uint64(m.GetNumber("term").Long()),
				Digest: m.GetString("digest").Raw(),
				Unix:   m.GetNumber("unix").Long(),
			},
			nodeID:    m.GetString("node_id").String(),
			signature: m.GetString("signature").Raw(),
			cert:      m.GetString("cert").Raw(),
		}, nil
	},
}

func (t *LedgerService) digest(ctx context.Context, _ value.Value) (*digestResponse, error) {
	if err := t.Policy.Authorize(ctx, iam.PermProofsRead, "", ""); err != nil {
		return nil, err
	}
	index, digest := t.FSM.ChainHead()
	ckpt := &ledger.Checkpoint{Height: index, Term: 0, Digest: digest[:], Unix: time.Now().Unix()}

	resp := &digestResponse{checkpoint: ckpt}
	if t.signer != nil {
		ckpt.Term = 0
		resp.nodeID = t.signer.NodeID()
		resp.signature = t.signer.Sign(ckpt)
		if raw, err := ledger.EncodeCert(t.signer.Cert()); err == nil {
			resp.cert = raw
		}
	}
	return resp, nil
}
