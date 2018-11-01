package bbclient

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
	"strconv"
)

type IBigBagger interface {
	CreateDataset(props map[string]string) error

	UpdateDataset(props map[string]string) error

	DeleteDataset(name string) error

	GetDatasetStatus(name string) (map[string]string, error)

	ListDatasets() ([]string, error)

	Execute(IOperation) (IResult, error)

	ExecuteList([]IOperation) ([]IResult, error)
}

type BigBaggerClient struct {
	token           string
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

func (this *BigBaggerClient) CreateDataset(props map[string]string) (err error) {

	request := new(bbproto.CreateDatasetRequest)
	request.Token = this.token

	request.Dataset, err = ParseDatasetProps(props)
	if err != nil {
		return err
	}

	_, err = this.datasetService.Create(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) UpdateDataset(props map[string]string) (err error) {

	request := new(bbproto.UpdateDatasetRequest)
	request.Token = this.token

	request.Dataset, err = ParseDatasetProps(props)
	if err != nil {
		return err
	}

	_, err = this.datasetService.Update(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}


func ParseDatasetProps(props map[string]string) (*bbproto.Dataset, error) {

	dataset := new(bbproto.Dataset)
	dataset.Name = props["name"]

	distr, ok := props["distr"]
	if !ok {
		return nil, errors.New("empty distr prop")
	}

	distrCode, ok := bbproto.DataDistribution_value[distr]
	if !ok {
		return nil, errors.New("wrong distr prop")
	}

	dataset.Distr = bbproto.DataDistribution(distrCode)

	if dataset.Distr == bbproto.DataDistribution_PARTITION {

		total := props["partition.total"]

		totalNum, err := strconv.ParseInt(total, 10, 32)
		if err != nil {
			return nil, errors.New("wrong partition.total prop")
		}

		replicationFactor := props["partition.replicationFactor"]

		replicationFactorNum, err := strconv.ParseInt(replicationFactor, 10, 32)
		if err != nil {
			return nil, errors.New("wrong partition.replicationFactor prop")
		}

		dataset.Partition.Total = int32(totalNum)
		dataset.Partition.ReplicationFactor = int32(replicationFactorNum)

	}

	dataset.Encryption = props["encryption"]
	dataset.Compression = props["compression"]

	return dataset, nil
}

func (this *BigBaggerClient) DeleteDataset(name string) error {

	request := new(bbproto.DeleteDatasetRequest)
	request.Token = this.token
	request.Name = name

	_, err := this.datasetService.Delete(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) GetDatasetStatus(name string) (status map[string]string, err error) {

	request := new(bbproto.GetDatasetStatusRequest)
	request.Token = this.token
	request.Name = name

	response, err := this.datasetService.Status(context.Background(), request)

	if err != nil {
		return nil, err
	}

	status = make(map[string]string)

	status["name"] = response.Dataset.Name
	status["distr"] = response.Dataset.Distr.String()
	if response.Dataset.Distr == bbproto.DataDistribution_PARTITION {
		status["partition.total"] = string(response.Dataset.Partition.Total)
		status["partition.replicationFactor"] = string(response.Dataset.Partition.ReplicationFactor)
	}
	status["encryption"] = response.Dataset.Encryption
	status["compression"] = response.Dataset.Compression

	return status, nil

}

func (this *BigBaggerClient) ListDatasets() (list []string, err error) {

	request := new(bbproto.ListDatasetsRequest)
	request.Token = this.token
	response, err := this.datasetService.List(context.Background(), request)

	if err != nil {
		return nil, err
	}

	return response.Name, nil

}

func (this *BigBaggerClient) Execute(op IOperation) (res IResult, err error) {

	request := new(bbproto.RecordRequest)
	request.Token = this.token
	request.List = make([]*bbproto.RecordOpeation, 1)

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
	request.Token = this.token
	request.List = make([]*bbproto.RecordOpeation, size)

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

	var cli = &BigBaggerClient{token, conn, bbproto.NewDatasetServiceClient(conn), bbproto.NewRecordServiceClient(conn)}

	return cli, nil
}

