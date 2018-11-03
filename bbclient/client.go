package bbclient

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
)

type IBigBagger interface {
	CreateDataset(dataset *bbproto.Dataset) error

	UpdateDataset(dataset *bbproto.Dataset) error

	DeleteDataset(name string) error

	GetDataset(pattern string) ([]*bbproto.Dataset, error)

	Execute(IOperation) (IResult, error)

	ExecuteTransaction([]IOperation) ([]IResult, error)
}

type BigBaggerClient struct {
	conn                 *grpc.ClientConn
	datasetService       bbproto.DatasetServiceClient
	transactionService   bbproto.TransactionServiceClient

}

func (cli *BigBaggerClient) Close() error {
	if conn := cli.conn; conn != nil {
		cli.conn = nil
		return conn.Close()
	}
	return nil
}

func (this *BigBaggerClient) CreateDataset(dataset *bbproto.Dataset) (err error) {

	_, err = this.datasetService.Create(context.Background(), dataset)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) UpdateDataset(dataset *bbproto.Dataset) (err error) {

	_, err = this.datasetService.Update(context.Background(), dataset)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) DeleteDataset(name string) error {

	request := new(bbproto.String)
	request.Value = name

	_, err := this.datasetService.Delete(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) GetDataset(pattern string) (result []*bbproto.Dataset, err error) {

	request := new(bbproto.String)
	request.Value = pattern

	response, err := this.datasetService.Get(context.Background(), request)

	if err != nil {
		return nil, err
	}

	result = make([]*bbproto.Dataset, 0, 10)

	for dataset, e := response.Recv(); e == nil; dataset, e = response.Recv() {
		result = append(result, dataset)
	}

	return result, err

}

func (this *BigBaggerClient) Execute(op IOperation) (res IResult, err error) {

	request := new(bbproto.Transaction)
	request.Operations = make([]*bbproto.RecordOperation, 1)

	request.Operations[0] = op.toProto()

	response, err := this.transactionService.Execute(context.Background(), request)

	if err != nil {
		return nil, err
	}

	if len(response.Results) != 1 {
		return nil, errors.New("expected response with 1 result")
	}

	return ParseResult(response.Results[0]), nil

}

func (this *BigBaggerClient) ExecuteTransaction(ops []IOperation) (res []IResult, err error) {

	size := len(ops)

	request := new(bbproto.Transaction)
	request.Operations = make([]*bbproto.RecordOperation, size)

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
	bbproto.NewDatasetServiceClient(conn),
	 bbproto.NewTransactionServiceClient(conn)}

	return cli, nil
}

