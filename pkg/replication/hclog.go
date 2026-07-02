/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"os"
	"reflect"

	"github.com/hashicorp/go-hclog"
	"go.arpabet.com/glue"
)

/*
HCLogFactory provides the hclog.Logger that hashicorp/raft (via raftmod) requires.
We do not pull in sprintframework just for its logger; a plain hclog writing to
stderr is sufficient and keeps the dependency surface small.
*/

var hclogClass = reflect.TypeOf((*hclog.Logger)(nil)).Elem()

type implHCLogFactory struct {
	Level string `value:"raft.log-level,default=INFO"`
}

func HCLogFactory() glue.FactoryBean { return &implHCLogFactory{} }

func (t *implHCLogFactory) Object() (interface{}, error) {
	return hclog.New(&hclog.LoggerOptions{
		Name:   "raft",
		Level:  hclog.LevelFromString(t.Level),
		Output: os.Stderr,
	}), nil
}

func (t *implHCLogFactory) ObjectType() reflect.Type { return hclogClass }

func (t *implHCLogFactory) ObjectName() string { return "hclog" }

func (t *implHCLogFactory) Singleton() bool { return true }
