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


package cserver

import (
	"gopkg.in/yaml.v2"
	"path/filepath"
	"io/ioutil"
	"github.com/pkg/errors"
	"fmt"
	"runtime"
	"github.com/consensusdb/consensusdb/cdb"
)

type Configuration struct {

	Host           		   string  `yaml:"host"`
	HttpPort    	       int     `yaml:"httpPort"`
	GrpcPort    	       int     `yaml:"grpcPort"`

	HttpAddress            string
	GrpcAddress            string

	DataDir                string  `yaml:"DataDir"`

	KeyDir                 string
	ValueDir               string
	WalDir                 string
	SnapDir                string
	LogDir                 string

	NumCPU				   int     `yaml:"NumCPU"`    // use all of <= 0

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

	err = initialize(conf)
	return conf, err
}


func initialize(conf *Configuration) error {

	if len(conf.Host) == 0 {
		return errors.New("host is empty")
	}

	conf.HttpAddress = fmt.Sprint(conf.Host, ":", conf.HttpPort)
	conf.GrpcAddress = fmt.Sprint(conf.Host, ":", conf.GrpcPort)

	if len(conf.DataDir) == 0 {
		return errors.New("dataDir is empty")
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