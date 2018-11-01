package bbclient

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"bigbagger/proto/bbproto"
	"github.com/pkg/errors"
	"strconv"
)

type IBigBagger interface {
	CreateDataset(name string, props map[string]string) error

	UpdateDataset(name string, props map[string]string) error

	DeleteDataset(name string, props map[string]string) error

	GetDatasetStatus(name string) (map[string]string, error)

	ListDatasets() ([]string, error)
}

type BigBaggerClient struct {
	conn   *grpc.ClientConn
	client bbproto.BigBaggerServiceClient
}

func (cli *BigBaggerClient) Close() error {
	if conn := cli.conn; conn != nil {
		cli.conn = nil
		return conn.Close()
	}
	return nil
}

func (this *BigBaggerClient) CreateDataset(name string, props map[string]string) error {

	request := &bbproto.CreateDatasetRequest{}
	request.Dataset.Name = name

	distr, ok := props["distr"]
	if !ok {
		return errors.New("empty distr prop")
	}

	distrCode, ok := bbproto.DataDistribution_value[distr]
	if !ok {
		return errors.New("wrong distr prop")
	}

	request.Dataset.Distr = bbproto.DataDistribution(distrCode)

	if request.Dataset.Distr == bbproto.DataDistribution_PARTITION {

		total := props["partition.total"]

		totalNum, err := strconv.ParseInt(total, 10, 32)
		if err != nil {
			return errors.New("wrong partition.total prop")
		}

		replicationFactor := props["partition.replicationFactor"]

		replicationFactorNum, err := strconv.ParseInt(replicationFactor, 10, 32)
		if err != nil {
			return errors.New("wrong partition.replicationFactor prop")
		}

		request.Dataset.Partition.Total = int32(totalNum)
		request.Dataset.Partition.ReplicationFactor = int32(replicationFactorNum)

	}

	request.Dataset.Encryption = props["encryption"]
	request.Dataset.Compression = props["compression"]

	_, err := this.client.Create(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) UpdateDataset(name string, props map[string]string) error {

	request := &bbproto.UpdateDatasetRequest{}
	request.Dataset.Name = name

	_, err := this.client.Update(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) DeleteDataset(name string, props map[string]string) error {

	request := &bbproto.DeleteDatasetRequest{}
	request.Name = name

	_, err := this.client.Delete(context.Background(), request)

	if err != nil {
		return err
	}

	return nil

}

func (this *BigBaggerClient) GetDatasetStatus(name string) (status map[string]string, err error) {

	request := &bbproto.GetDatasetStatusRequest{}
	request.Name = name

	response, err := this.client.Status(context.Background(), request)

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

	request := &bbproto.ListDatasetsRequest{}

	response, err := this.client.List(context.Background(), request)

	if err != nil {
		return nil, err
	}

	return response.Name, nil

}


func NewClient(grpcAddress string) (*BigBaggerClient, error) {

	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	var cli = &BigBaggerClient{conn, bbproto.NewBigBaggerServiceClient(conn)}

	return cli, nil
}
