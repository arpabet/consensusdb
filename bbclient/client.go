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

	ExecuteList([]IOperation) ([]IResult, error)
}

type BigBaggerClient struct {
	conn            *grpc.ClientConn
	datasetService  bbproto.DatasetServiceClient
	recordService   bbproto.RecordServiceClient

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

	request := new(bbproto.Name)
	request.Name = name

	_, err := this.datasetService.Delete(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) GetDataset(pattern string) (result []*bbproto.Dataset, err error) {

	request := new(bbproto.Name)
	request.Name = pattern

	response, err := this.datasetService.Get(context.Background(), request)

	if err != nil {
		return nil, err
	}

	result = make([]*bbproto.Dataset, 0, 10)

	for dataset, err := response.Recv(); err != nil; dataset, err = response.Recv() {

		result = append(result, dataset)

	}

	return result, err

}

func (this *BigBaggerClient) Execute(op IOperation) (res IResult, err error) {

	request := new(bbproto.RecordRequest)
	request.List = make([]*bbproto.RecordOperation, 1)

	request.List[0] = op.toProto()

	response, err := this.recordService.Execute(context.Background(), request)

	if err != nil {
		return nil, err
	}

	if len(response.List) != 1 {
		return nil, errors.New("expected response with 1 result")
	}

	return ParseResult(response.List[0]), nil

}

func (this *BigBaggerClient) ExecuteList(ops []IOperation) (res []IResult, err error) {

	size := len(ops)

	request := new(bbproto.RecordRequest)
	request.List = make([]*bbproto.RecordOperation, size)

	for i, op := range ops {
		request.List[i] = op.toProto()
	}

	response, err := this.recordService.Execute(context.Background(), request)

	if err != nil {
		return nil, err
	}

	if size != len(response.List) {
		return nil, errors.New("wrong response size")
	}

	res = make([]IResult, size)

	for i, v := range response.List {
		res[i] = ParseResult(v)
	}

	return res, nil

}

func NewClient(grpcAddress, token string) (*BigBaggerClient, error) {

	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	var cli = &BigBaggerClient{conn, bbproto.NewDatasetServiceClient(conn), bbproto.NewRecordServiceClient(conn)}

	return cli, nil
}

