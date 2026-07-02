/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"github.com/pkg/errors"
	"go.arpabet.com/consensusdb/pkg/pb"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
)

/*
Replicated write commands are encoded into raft log entries as a single op byte
followed by the protobuf-marshaled payload. The FSM decodes them on every node
and applies them to local storage, so the encoding must stay stable on disk.
*/

type opCode byte

const (
	opPut       opCode = 1 // payload: pb.RecordRequest
	opTouch     opCode = 2 // payload: pb.RecordRequest
	opRemove    opCode = 3 // payload: pb.KeyRequest
	opIncrement opCode = 4 // payload: pb.IncrementRequest
	opBatch     opCode = 5 // payload: pb.BatchRequest
	opReclaim   opCode = 6 // payload: pb.ReclaimRequest
)

func encodeCommand(op opCode, msg proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "marshal raft command payload")
	}
	buf := make([]byte, 0, len(payload)+1)
	buf = append(buf, byte(op))
	buf = append(buf, payload...)
	return buf, nil
}

func decodeCommand(data []byte) (opCode, proto.Message, error) {
	if len(data) == 0 {
		return 0, nil, xerrors.New("empty raft command")
	}
	op := opCode(data[0])
	payload := data[1:]
	var msg proto.Message
	switch op {
	case opPut, opTouch:
		msg = &pb.RecordRequest{}
	case opRemove:
		msg = &pb.KeyRequest{}
	case opIncrement:
		msg = &pb.IncrementRequest{}
	case opBatch:
		msg = &pb.BatchRequest{}
	case opReclaim:
		msg = &pb.ReclaimRequest{}
	default:
		return op, nil, xerrors.Errorf("unknown raft command op %d", op)
	}
	if err := proto.Unmarshal(payload, msg); err != nil {
		return op, nil, errors.Wrap(err, "unmarshal raft command payload")
	}
	return op, msg, nil
}
