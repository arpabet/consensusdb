/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package server

import (
	"bytes"
	"context"
	"sort"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.arpabet.com/store"
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
	// Reads are served locally (any node); writes route through the Replicator and
	// may return NotLeaderError, wrapped so the client can redirect to the leader.
	must(valueserver.AddUnary(t.Server, "kv.get", keyRequestCodec, recordCodec, t.svc.Get))
	must(valueserver.AddUnary(t.Server, "kv.getrecent", keyRequestCodec, recordCodec, t.svc.GetRecent))
	must(valueserver.AddUnary(t.Server, "kv.put", recordRequestCodec, statusCodec, redirectable(t.svc.Put)))
	must(valueserver.AddUnary(t.Server, "kv.touch", recordRequestCodec, statusCodec, redirectable(t.svc.Touch)))
	must(valueserver.AddUnary(t.Server, "kv.remove", keyRequestCodec, statusCodec, redirectable(t.svc.Remove)))
	must(valueserver.AddUnary(t.Server, "kv.increment", incrementRequestCodec, incrementResponseCodec, redirectable(t.svc.Increment)))
	must(valueserver.AddUnary(t.Server, "kv.batch", batchRequestCodec, statusCodec, redirectable(t.svc.Batch)))
	// Server-streams: enumerate records under a prefix, and watch changes.
	must(valueserver.AddOutgoingStreamTyped(t.Server, "kv.enumerate", enumerateRequestCodec, recordCodec, t.enumerate))
	must(valueserver.AddOutgoingStreamTyped(t.Server, "kv.watch", watchRequestCodec, watchEventCodec, t.watch))
	return nil
}

// NotLeaderPrefix marks a value-rpc error meaning "not leader; redirect". The
// message after the prefix is the leader's value-rpc endpoint (host:port).
const NotLeaderPrefix = "not-leader:"

// redirectable wraps a write handler so a NotLeaderError becomes a Unavailable
// value-rpc error carrying the leader endpoint, letting the client redirect.
func redirectable[Req, Resp any](fn func(context.Context, Req) (Resp, error)) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, req Req) (Resp, error) {
		resp, err := fn(ctx, req)
		if nl, ok := AsNotLeader(err); ok {
			var zero Resp
			return zero, valuerpc.NewError(valuerpc.CodeUnavailable, "%s%s", NotLeaderPrefix, nl.LeaderEndpoint)
		}
		return resp, err
	}
}

// enumerate streams records under the request's prefix (depth inferred from the
// set fields; empty prefix scans everything).
//
// Unordered (default): records stream as the engine yields them (consensusdb's
// length-prefixed key order, NOT lexical), bounded by credit-based flow control.
// Ordered (opt-in): the region's records are buffered and sorted by the decoded
// minor key (lexical, reversed on request) before streaming — O(n) server memory,
// which is why it is opt-in. This is what lets the cdb provider advertise Ordered.
func (t *VrpcDataService) enumerate(ctx context.Context, req *pb.EnumerateRequest) (<-chan *pb.Record, error) {
	out := make(chan *pb.Record)

	scan := func(sender BlockSender) error {
		if field, ok := areaField(req.Prefix); ok {
			return t.Storage.GetArea(&pb.KeyRequest{Key: req.Prefix}, field, sender)
		}
		return t.Storage.Scan(&pb.ScanRequest{}, sender)
	}

	go func() {
		defer close(out)

		if !req.Ordered {
			if err := scan(&chanBlockSender{ctx: ctx, out: out}); err != nil && ctx.Err() == nil {
				t.Log.Error("VrpcEnumerate", zap.Error(err))
			}
			return
		}

		coll := &collectSender{}
		if err := scan(coll); err != nil {
			t.Log.Error("VrpcEnumerate", zap.Error(err))
			return
		}
		sort.Slice(coll.records, func(i, j int) bool {
			c := bytes.Compare(coll.records[i].Key.MinorKey, coll.records[j].Key.MinorKey)
			if req.Reverse {
				return c > 0
			}
			return c < 0
		})
		for _, rec := range coll.records {
			select {
			case out <- rec:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// collectSender buffers records for server-side ordering.
type collectSender struct{ records []*pb.Record }

func (s *collectSender) Send(block *pb.Block) error {
	s.records = append(s.records, block.Record...)
	return nil
}

// watch streams change events for keys under the request's prefix. Best-effort:
// the underlying WatchHub drops events for a watcher that falls behind.
func (t *VrpcDataService) watch(ctx context.Context, req *pb.WatchRequest) (<-chan *pb.WatchEvent, error) {
	prefix, err := watchPrefix(req.Prefix)
	if err != nil {
		return nil, err
	}
	out := make(chan *pb.WatchEvent)
	go func() {
		defer close(out)
		_ = t.Storage.WatchRaw(ctx, prefix, func(ev *store.WatchEvent) bool {
			key, derr := DecodeKey(ev.Key)
			if derr != nil {
				return true // skip undecodable, keep watching
			}
			changeType := pb.ChangeType_WATCH_SET
			if ev.Type == store.WatchDelete {
				changeType = pb.ChangeType_WATCH_DELETE
			}
			select {
			case out <- &pb.WatchEvent{Key: key, Value: ev.Value, Version: uint64(ev.Version), Type: changeType}:
				return true
			case <-ctx.Done():
				return false
			}
		})
	}()
	return out, nil
}

// chanBlockSender adapts the push-based BlockSender to a channel, honoring ctx
// cancellation so a disconnected client stops the underlying scan.
type chanBlockSender struct {
	ctx context.Context
	out chan<- *pb.Record
}

func (s *chanBlockSender) Send(block *pb.Block) error {
	for _, rec := range block.Record {
		select {
		case s.out <- rec:
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
	return nil
}

// areaField picks the GetArea depth from the set fields of a prefix Key; false
// means no prefix (enumerate everything via Scan).
func areaField(key *pb.Key) (Field, bool) {
	if key == nil {
		return 0, false
	}
	switch {
	case len(key.MinorKey) > 0:
		return MinorKeyField, true
	case len(key.RegionName) > 0:
		return RegionNameField, true
	case len(key.MajorKey) > 0:
		return MajorKeyField, true
	default:
		return 0, false
	}
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

// EnumerateStream opens a record stream under the request's prefix. receiveCap is
// the client-side receive buffer; read *errp after the channel closes for a
// decode/stream error.
func EnumerateStream(ctx context.Context, cli valueclient.Client, req *pb.EnumerateRequest, receiveCap int, errp *error) (<-chan *pb.Record, error) {
	ch, _, err := valueclient.GetStreamTyped(ctx, cli, "kv.enumerate", req, receiveCap, enumerateRequestCodec, recordCodec, errp)
	return ch, err
}

// WatchStream opens a change-event stream for keys under the request's prefix.
func WatchStream(ctx context.Context, cli valueclient.Client, req *pb.WatchRequest, receiveCap int, errp *error) (<-chan *pb.WatchEvent, error) {
	ch, _, err := valueclient.GetStreamTyped(ctx, cli, "kv.watch", req, receiveCap, watchRequestCodec, watchEventCodec, errp)
	return ch, err
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

var watchRequestCodec = valuerpc.Codec[*pb.WatchRequest]{
	Encode: func(r *pb.WatchRequest) value.Value {
		if r == nil {
			r = &pb.WatchRequest{}
		}
		return value.EmptyMap(true).Put("prefix", encodeKey(r.Prefix))
	},
	Decode: func(v value.Value) (*pb.WatchRequest, error) {
		m, err := asMap(v, "watch request")
		if err != nil {
			return nil, err
		}
		return &pb.WatchRequest{Prefix: decodeKey(m.GetMap("prefix"))}, nil
	},
}

var watchEventCodec = valuerpc.Codec[*pb.WatchEvent]{
	Encode: func(e *pb.WatchEvent) value.Value {
		if e == nil {
			e = &pb.WatchEvent{}
		}
		return value.EmptyMap(true).
			Put("key", encodeKey(e.Key)).
			Put("value", value.Raw(e.Value, false)).
			Put("version", value.Long(int64(e.Version))).
			Put("type", value.Long(int64(e.Type)))
	},
	Decode: func(v value.Value) (*pb.WatchEvent, error) {
		m, err := asMap(v, "watch event")
		if err != nil {
			return nil, err
		}
		return &pb.WatchEvent{
			Key:     decodeKey(m.GetMap("key")),
			Value:   m.GetString("value").Raw(),
			Version: uint64(m.GetNumber("version").Long()),
			Type:    pb.ChangeType(m.GetNumber("type").Long()),
		}, nil
	},
}

var enumerateRequestCodec = valuerpc.Codec[*pb.EnumerateRequest]{
	Encode: func(r *pb.EnumerateRequest) value.Value {
		if r == nil {
			r = &pb.EnumerateRequest{}
		}
		return value.EmptyMap(true).
			Put("prefix", encodeKey(r.Prefix)).
			Put("ordered", value.Boolean(r.Ordered)).
			Put("reverse", value.Boolean(r.Reverse))
	},
	Decode: func(v value.Value) (*pb.EnumerateRequest, error) {
		m, err := asMap(v, "enumerate request")
		if err != nil {
			return nil, err
		}
		return &pb.EnumerateRequest{
			Prefix:  decodeKey(m.GetMap("prefix")),
			Ordered: m.GetBool("ordered").Boolean(),
			Reverse: m.GetBool("reverse").Boolean(),
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
