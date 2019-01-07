/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */


package cdb

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"github.com/pkg/errors"
)

type IConsensusDB interface {

	Execute(IOperation) IResult

	ExecuteTransaction(operations []IOperation) ([]IResult, error)
}

type DefaultClient struct {
	conn               *grpc.ClientConn
	databaseService cserverpb.DatabaseServiceClient
}

func (cli *DefaultClient) Close() error {
	if conn := cli.conn; conn != nil {
		cli.conn = nil
		return conn.Close()
	}
	return nil
}

func (this *DefaultClient) Execute(op IOperation) (res IResult) {

	request := new(cserverpb.Transaction)
	request.Operations = make([]*cserverpb.TxOperation, 1)

	request.Operations[0] = op.toProto()

	response, err := this.databaseService.Execute(context.Background(), request)

	if err != nil {
		return NewErrorResult(cserverpb.StatusCode_ERROR_NETWORK, err.Error())
	}

	if len(response.Results) != 1 {
		return NewErrorResult(cserverpb.StatusCode_ERROR_NETWORK, "expected single result")
	}

	return ParseResult(response.Results[0])

}

func (this *DefaultClient) ExecuteTransaction(ops []IOperation) (res []IResult, err error) {

	size := len(ops)

	request := new(cserverpb.Transaction)
	request.Operations = make([]*cserverpb.TxOperation, size)

	for i, op := range ops {
		request.Operations[i] = op.toProto()
	}

	response, err := this.databaseService.Execute(context.Background(), request)

	if err != nil {
		return nil, err
	}

	if size != len(response.Results) {
		return nil, errors.New("wrong response size")
	}

	res = make([]IResult, size)

	for i, v := range response.Results {
		res[i] = ParseResult(v)
	}

	return res, nil

}

func NewClient(grpcAddress string) (*DefaultClient, error) {

	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	var cli = &DefaultClient{conn,
	 cserverpb.NewDatabaseServiceClient(conn)}

	return cli, nil
}
