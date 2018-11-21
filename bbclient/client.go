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


package bbclient

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"github.com/bigbagger/bigbagger/proto/bbproto"
	"github.com/pkg/errors"
)

type IBigBagger interface {
	CreateRegion(region *bbproto.Region) error

	UpdateRegion(region *bbproto.Region) error

	DeleteRegion(name string) error

	GetRegions(pattern string) ([]*bbproto.Region, error)

	Execute(IOperation) IResult

	ExecuteTransaction([]IOperation) ([]IResult, error)
}

type BigBaggerClient struct {
	conn                 *grpc.ClientConn
	regionService        bbproto.RegionServiceClient
	transactionService   bbproto.TransactionServiceClient

}

func (cli *BigBaggerClient) Close() error {
	if conn := cli.conn; conn != nil {
		cli.conn = nil
		return conn.Close()
	}
	return nil
}

func (this *BigBaggerClient) CreateRegion(region *bbproto.Region) (err error) {

	_, err = this.regionService.Create(context.Background(), region)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) UpdateRegion(region *bbproto.Region) (err error) {

	_, err = this.regionService.Update(context.Background(), region)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) DeleteRegion(name string) error {

	request := new(bbproto.String)
	request.Value = name

	_, err := this.regionService.Delete(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) GetRegions(pattern string) (result []*bbproto.Region, err error) {

	request := new(bbproto.String)
	request.Value = pattern

	response, err := this.regionService.Get(context.Background(), request)

	if err != nil {
		return nil, err
	}

	result = make([]*bbproto.Region, 0, 10)

	for dataset, e := response.Recv(); e == nil; dataset, e = response.Recv() {
		result = append(result, dataset)
	}

	return result, err

}

func (this *BigBaggerClient) Execute(op IOperation) (res IResult) {

	request := new(bbproto.Transaction)
	request.Operations = make([]*bbproto.TxOperation, 1)

	request.Operations[0] = op.toProto()

	response, err := this.transactionService.Execute(context.Background(), request)

	if err != nil {
		return NewNetworkError(err)
	}

	if len(response.Results) != 1 {
		return &ErrorResult{bbproto.StatusCode_ERROR_NETWORK, "expected single result"}
	}

	return ParseResult(response.Results[0])

}

func (this *BigBaggerClient) ExecuteTransaction(ops []IOperation) (res []IResult, err error) {

	size := len(ops)

	request := new(bbproto.Transaction)
	request.Operations = make([]*bbproto.TxOperation, size)

	for i, op := range ops {
		request.Operations[i] = op.toProto()
	}

	response, err := this.transactionService.Execute(context.Background(), request)

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

func NewClient(grpcAddress string) (*BigBaggerClient, error) {

	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	var cli = &BigBaggerClient{conn,
	bbproto.NewRegionServiceClient(conn),
	 bbproto.NewTransactionServiceClient(conn)}

	return cli, nil
}

