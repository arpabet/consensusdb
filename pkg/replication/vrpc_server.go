/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"go.arpabet.com/glue"
	"go.arpabet.com/value-rpc/valueserver"
	"go.uber.org/zap"
)

var vrpcServerClass = reflect.TypeOf((*valueserver.Server)(nil)).Elem()

var memServerCounter int64

type implVrpcServerFactory struct {
	Log        *zap.Logger     `inject:""`
	Properties glue.Properties `inject:""`

	beanName string
}

/*
VrpcServerFactory produces a valueserver.Server bean named beanName, listening on
the address in <beanName>.bind-address ("tcp://host:port" or "unix:///path").
Handlers register on it via the returned bean (AddFunction is concurrent-safe, so
the server runs immediately and the raft control service registers afterward).

An empty bind-address disables the vrpc endpoint on this node: the factory falls
back to a uniquely-named in-process mem server, so the bean graph still resolves
(the control server and pool inject cleanly) but nothing binds to a port.
*/
func VrpcServerFactory(beanName string) glue.FactoryBean {
	return &implVrpcServerFactory{beanName: beanName}
}

func (t *implVrpcServerFactory) Object() (interface{}, error) {
	address := t.Properties.GetString(fmt.Sprintf("%s.bind-address", t.beanName), "")

	if address == "" {
		name := fmt.Sprintf("%s-disabled-%d", t.beanName, atomic.AddInt64(&memServerCounter, 1))
		srv, err := valueserver.NewMemServer(name, t.Log)
		if err != nil {
			return nil, err
		}
		go srv.Run()
		t.Log.Info("VrpcServerDisabled", zap.String("bean", t.beanName), zap.String("mem", name))
		return srv, nil
	}

	srv, err := valueserver.NewServer(address, t.Log)
	if err != nil {
		return nil, err
	}
	go srv.Run()
	t.Log.Info("VrpcServerFactory", zap.String("bean", t.beanName), zap.String("address", address))
	return srv, nil
}

func (t *implVrpcServerFactory) ObjectType() reflect.Type { return vrpcServerClass }
func (t *implVrpcServerFactory) ObjectName() string       { return t.beanName }
func (t *implVrpcServerFactory) Singleton() bool          { return true }
