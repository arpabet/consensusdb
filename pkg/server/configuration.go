/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */


package server

import (
	"gopkg.in/yaml.v2"
	"path/filepath"
	"io/ioutil"
	"github.com/pkg/errors"
	"fmt"
	"runtime"
	"go.arpabet.com/consensusdb/cdb"
	"os"
)

type Configuration struct {

	Host           		   string  `yaml:"host"`
	HttpPort    	       int     `yaml:"httpPort"`
	GrpcPort    	       int     `yaml:"grpcPort"`

	HttpAddress            string
	GrpcAddress            string

	DataDir                string  `yaml:"dataDir"`

	KeyDir                 string
	ValueDir               string
	WalDir                 string
	SnapDir                string
	LogDir                 string

	FileIO                 bool    `yaml:"fileIO"`

	NumCPU				   int     `yaml:"numCPU"`    // use all of <= 0

}

func LoadConfiguration(fileName string) (conf *Configuration, err error) {

	conf = new(Configuration)

	yamlFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Errorf("config load error %q", err)
	}

	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		return nil, errors.Errorf("config unmarshal error %q", err)
	}

	err = initialize(conf)
	return conf, err
}


func NewDefaultConfiguration(dataDir string) (conf *Configuration, err error) {

	conf = new(Configuration)

	conf.Host = "localhost"
	conf.HttpPort = 8441
	conf.GrpcPort = 8442

	conf.DataDir = dataDir
	conf.FileIO = true

	err = initialize(conf)
	return conf, err
}


func initialize(conf *Configuration) error {

	if len(conf.Host) == 0 {
		return errors.New("yaml: host is empty")
	}

	conf.HttpAddress = fmt.Sprint(conf.Host, ":", conf.HttpPort)
	conf.GrpcAddress = fmt.Sprint(conf.Host, ":", conf.GrpcPort)

	if len(conf.DataDir) == 0 {
		return errors.New("yaml: dataDir is empty")
	}

	if _, err := os.Stat(conf.DataDir); os.IsNotExist(err) {
		return errors.Errorf("dataDir is not exist: %s", conf.DataDir)
	}

	conf.KeyDir = filepath.Join(conf.DataDir, "key")
	conf.ValueDir = filepath.Join(conf.DataDir, "value")
	conf.WalDir = filepath.Join(conf.DataDir, "WAL")
	conf.SnapDir = filepath.Join(conf.DataDir, "snapshot")
	conf.LogDir = filepath.Join(conf.DataDir, "log")

	if conf.NumCPU <= 0 {
		conf.NumCPU = runtime.NumCPU()
	}

	return cdb.CreateDirsIfNotExist(conf.KeyDir, conf.ValueDir, conf.WalDir, conf.SnapDir, conf.LogDir)
}