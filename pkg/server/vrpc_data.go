/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"context"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/value"
	"go.arpabet.com/value-rpc/valueclient"
	"go.arpabet.com/value-rpc/valueserver"
	"go.arpabet.com/value-rpc/valuerpc"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
VrpcDataService exposes the key-value data plane over value-rpc, alongside the
gRPC KeyValueService. It is a thin adapter: it decodes value.Map requests, calls
the same KeyValueService methods (so replication routing, NotLeaderError, and all
storage semantics are reused), and encodes the responses. Registered on the shared
vrpc server (see the vrpc-server host bean).

Unary today: get / getrecent / put (set; CAS via RecordRequest.compareAndSet) /
touch / remove / increment / batch. Enumerate and watch streams are added next.
*/
type VrpcDataService struct {
	Server     valueserver.Server `inject:""`
	Storage    KeyValueStorage    `inject:""`
	Replicator Replicator         `inject:"optional"`
	Log        *zap.Logger        `inject:""`

	svc *KeyValueService
}

func (t *VrpcDataService) BeanName() string { return "vrpc-data-service" }

func (t *VrpcDataService) PostConstruct() error {
	// Build the same service the gRPC scanner does over the shared Storage /
	// Replicator, so both wires route writes and reads identically.
	t.svc = &KeyValueService{Storage: t.Storage, Replicator: t.Replicator, Log: t.Log}

	must := func(err error) {
		if err != nil {
			t.Log.Error("VrpcDataRegister", zap.Error(err))
		}
	}
	must(valueserver.AddUnary(t.Server, "kv.get", keyRequestCodec, recordCodec, t.svc.Get))
	must(valueserver.AddUnary(t.Server, "kv.getrecent", keyRequestCodec, recordCodec, t.svc.GetRecent))
	must(valueserver.AddUnary(t.Server, "kv.put", recordRequestCodec, statusCodec, t.svc.Put))
	must(valueserver.AddUnary(t.Server, "kv.touch", recordRequestCodec, statusCodec, t.svc.Touch))
	must(valueserver.AddUnary(t.Server, "kv.remove", keyRequestCodec, statusCodec, t.svc.Remove))
	must(valueserver.AddUnary(t.Server, "kv.increment", incrementRequestCodec, incrementResponseCodec, t.svc.Increment))
	must(valueserver.AddUnary(t.Server, "kv.batch", batchRequestCodec, statusCodec, t.svc.Batch))
	return nil
}

// --- typed client helpers (value-rpc data plane) -----------------------------

func CallGet(ctx context.Context, cli valueclient.Client, req *pb.KeyRequest) (*pb.Record, error) {
	return valueclient.CallUnary(ctx, cli, "kv.get", req, keyRequestCodec, recordCodec)
}
func CallGetRecent(ctx context.Context, cli valueclient.Client, req *pb.KeyRequest) (*pb.Record, error) {
	return valueclient.CallUnary(ctx, cli, "kv.getrecent", req, keyRequestCodec, recordCodec)
}
func CallPut(ctx context.Context, cli valueclient.Client, req *pb.RecordRequest) (*pb.Status, error) {
	return valueclient.CallUnary(ctx, cli, "kv.put", req, recordRequestCodec, statusCodec)
}
func CallTouch(ctx context.Context, cli valueclient.Client, req *pb.RecordRequest) (*pb.Status, error) {
	return valueclient.CallUnary(ctx, cli, "kv.touch", req, recordRequestCodec, statusCodec)
}
func CallRemove(ctx context.Context, cli valueclient.Client, req *pb.KeyRequest) (*pb.Status, error) {
	return valueclient.CallUnary(ctx, cli, "kv.remove", req, keyRequestCodec, statusCodec)
}
func CallIncrement(ctx context.Context, cli valueclient.Client, req *pb.IncrementRequest) (*pb.IncrementResponse, error) {
	return valueclient.CallUnary(ctx, cli, "kv.increment", req, incrementRequestCodec, incrementResponseCodec)
}
func CallBatch(ctx context.Context, cli valueclient.Client, req *pb.BatchRequest) (*pb.Status, error) {
	return valueclient.CallUnary(ctx, cli, "kv.batch", req, batchRequestCodec, statusCodec)
}

// --- value.Map <-> pb codecs -------------------------------------------------

func encodeKey(k *pb.Key) value.Value {
	if k == nil {
		return value.EmptyMap(true)
	}
	m := value.EmptyMap(true).
		Put("major", value.Raw(k.MajorKey, false)).
		Put("region", value.Raw(k.RegionName, false)).
		Put("minor", value.Raw(k.MinorKey, false))
	if k.Timestamp != nil {
		m = m.Put("ts_most", value.Long(k.Timestamp.MostSigBits)).
			Put("ts_least", value.Long(k.Timestamp.LeastSigBits)).
			Put("ts", value.Boolean(true))
	}
	return m
}

func decodeKey(m value.Map) *pb.Key {
	if m == nil {
		return nil
	}
	k := &pb.Key{
		MajorKey:   m.GetString("major").Raw(),
		RegionName: m.GetString("region").Raw(),
		MinorKey:   m.GetString("minor").Raw(),
	}
	if m.GetBool("ts").Boolean() {
		k.Timestamp = &pb.TimeUUID{
			MostSigBits:  m.GetNumber("ts_most").Long(),
			LeastSigBits: m.GetNumber("ts_least").Long(),
		}
	}
	return k
}

func asMap(v value.Value, what string) (value.Map, error) {
	m, ok := v.(value.Map)
	if !ok {
		return nil, xerrors.Errorf("%s: expected a map", what)
	}
	return m, nil
}

var keyRequestCodec = valuerpc.Codec[*pb.KeyRequest]{
	Encode: func(r *pb.KeyRequest) value.Value {
		if r == nil {
			r = &pb.KeyRequest{}
		}
		return value.EmptyMap(true).
			Put("key", encodeKey(r.Key)).
			Put("head_only", value.Boolean(r.HeadOnly))
	},
	Decode: func(v value.Value) (*pb.KeyRequest, error) {
		m, err := asMap(v, "key request")
		if err != nil {
			return nil, err
		}
		return &pb.KeyRequest{
			Key:      decodeKey(m.GetMap("key")),
			HeadOnly: m.GetBool("head_only").Boolean(),
		}, nil
	},
}

var recordRequestCodec = valuerpc.Codec[*pb.RecordRequest]{
	Encode: func(r *pb.RecordRequest) value.Value {
		if r == nil {
			r = &pb.RecordRequest{}
		}
		return value.EmptyMap(true).
			Put("key", encodeKey(r.Key)).
			Put("value", value.Raw(r.Value, false)).
			Put("metadata", value.Long(int64(r.Metadata))).
			Put("ttl", value.Long(r.TtlSeconds)).
			Put("cas", value.Boolean(r.CompareAndSet)).
			Put("version", value.Long(int64(r.Version))).
			Put("expires_at", value.Long(r.ExpiresAt))
	},
	Decode: func(v value.Value) (*pb.RecordRequest, error) {
		m, err := asMap(v, "record request")
		if err != nil {
			return nil, err
		}
		return &pb.RecordRequest{
			Key:           decodeKey(m.GetMap("key")),
			Value:         m.GetString("value").Raw(),
			Metadata:      int32(m.GetNumber("metadata").Long()),
			TtlSeconds:    m.GetNumber("ttl").Long(),
			CompareAndSet: m.GetBool("cas").Boolean(),
			Version:       uint64(m.GetNumber("version").Long()),
			ExpiresAt:     m.GetNumber("expires_at").Long(),
		}, nil
	},
}

var recordCodec = valuerpc.Codec[*pb.Record]{
	Encode: func(r *pb.Record) value.Value {
		if r == nil {
			r = &pb.Record{}
		}
		m := value.EmptyMap(true).
			Put("key", encodeKey(r.Key)).
			Put("value", value.Raw(r.Value, false)).
			Put("found", value.Boolean(r.Head != nil))
		if r.Head != nil {
			m = m.Put("version", value.Long(int64(r.Head.Version))).
				Put("expires_at", value.Long(int64(r.Head.ExpiresAt))).
				Put("disk_size", value.Long(r.Head.DiskSize)).
				Put("metadata", value.Long(int64(r.Head.Metadata)))
		}
		return m
	},
	Decode: func(v value.Value) (*pb.Record, error) {
		m, err := asMap(v, "record")
		if err != nil {
			return nil, err
		}
		rec := &pb.Record{
			Key:   decodeKey(m.GetMap("key")),
			Value: m.GetString("value").Raw(),
		}
		if m.GetBool("found").Boolean() {
			rec.Head = &pb.Head{
				Version:   uint64(m.GetNumber("version").Long()),
				ExpiresAt: uint64(m.GetNumber("expires_at").Long()),
				DiskSize:  m.GetNumber("disk_size").Long(),
				Metadata:  int32(m.GetNumber("metadata").Long()),
			}
		}
		return rec, nil
	},
}

var statusCodec = valuerpc.Codec[*pb.Status]{
	Encode: func(s *pb.Status) value.Value {
		if s == nil {
			s = &pb.Status{}
		}
		return value.EmptyMap(true).Put("updated", value.Boolean(s.Updated))
	},
	Decode: func(v value.Value) (*pb.Status, error) {
		m, err := asMap(v, "status")
		if err != nil {
			return nil, err
		}
		return &pb.Status{Updated: m.GetBool("updated").Boolean()}, nil
	},
}

var incrementRequestCodec = valuerpc.Codec[*pb.IncrementRequest]{
	Encode: func(r *pb.IncrementRequest) value.Value {
		if r == nil {
			r = &pb.IncrementRequest{}
		}
		return value.EmptyMap(true).
			Put("key", encodeKey(r.Key)).
			Put("initial", value.Long(r.Initial)).
			Put("delta", value.Long(r.Delta)).
			Put("ttl", value.Long(r.TtlSeconds)).
			Put("expires_at", value.Long(r.ExpiresAt))
	},
	Decode: func(v value.Value) (*pb.IncrementRequest, error) {
		m, err := asMap(v, "increment request")
		if err != nil {
			return nil, err
		}
		return &pb.IncrementRequest{
			Key:        decodeKey(m.GetMap("key")),
			Initial:    m.GetNumber("initial").Long(),
			Delta:      m.GetNumber("delta").Long(),
			TtlSeconds: m.GetNumber("ttl").Long(),
			ExpiresAt:  m.GetNumber("expires_at").Long(),
		}, nil
	},
}

var incrementResponseCodec = valuerpc.Codec[*pb.IncrementResponse]{
	Encode: func(r *pb.IncrementResponse) value.Value {
		if r == nil {
			r = &pb.IncrementResponse{}
		}
		return value.EmptyMap(true).
			Put("previous", value.Long(r.Previous)).
			Put("current", value.Long(r.Current)).
			Put("version", value.Long(int64(r.Version)))
	},
	Decode: func(v value.Value) (*pb.IncrementResponse, error) {
		m, err := asMap(v, "increment response")
		if err != nil {
			return nil, err
		}
		return &pb.IncrementResponse{
			Previous: m.GetNumber("previous").Long(),
			Current:  m.GetNumber("current").Long(),
			Version:  uint64(m.GetNumber("version").Long()),
		}, nil
	},
}

var batchRequestCodec = valuerpc.Codec[*pb.BatchRequest]{
	Encode: func(r *pb.BatchRequest) value.Value {
		list := value.EmptyList(true)
		if r != nil {
			for _, rec := range r.Records {
				list = list.Append(recordRequestCodec.Encode(rec))
			}
		}
		return value.EmptyMap(true).Put("records", list)
	},
	Decode: func(v value.Value) (*pb.BatchRequest, error) {
		m, err := asMap(v, "batch request")
		if err != nil {
			return nil, err
		}
		req := &pb.BatchRequest{}
		if list := m.GetList("records"); list != nil {
			for i := 0; i < list.Len(); i++ {
				rec, derr := recordRequestCodec.Decode(list.GetMapAt(i))
				if derr != nil {
					return nil, derr
				}
				req.Records = append(req.Records, rec)
			}
		}
		return req, nil
	},
}
