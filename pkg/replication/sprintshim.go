/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package replication

import (
	"context"
	"hash/fnv"
	"os"
	"strconv"
	"time"

	"go.arpabet.com/consensusdb/pkg/constants"
	"go.arpabet.com/uuid"
	"go.uber.org/zap"
)

/*
The raftmod beans were written for sprintframework and inject sprint.Application,
sprint.NodeService and sprint.SystemEnvironmentPropertyResolver. consensusdb runs
on the lighter cligo+servion stack, so this file provides minimal beans that
satisfy exactly those interfaces. They are intentionally thin: raftmod only calls
Application.Name(), NodeService.NodeIdHex() and NodeService.NodeSeq() at runtime;
the rest exist to satisfy the interface for dependency injection.

These beans implement the sprint interfaces structurally (no compile-time import
of the sprint package is required for that), so glue resolves them by interface.
*/

// ---------------------------------------------------------------------------
// Application
// ---------------------------------------------------------------------------

type Application struct {
	context.Context
	Name_    string `value:"application.name,default=consensusdb"`
	Profile_ string `value:"application.profile,default=prod"`
	DataDir_ string `value:"consensusdb.data-dir,default=/tmp/consensusdb"`
}

func NewApplication() *Application {
	return &Application{Context: context.Background()}
}

func (t *Application) PostConstruct() error {
	if t.Context == nil {
		t.Context = context.Background()
	}
	return nil
}

func (t *Application) BeanName() string { return "application" }

func (t *Application) GetStats(cb func(name, value string) bool) error {
	cb("name", t.Name_)
	cb("version", constants.GetAppInfo().Version)
	return nil
}

func (t *Application) AppendBeans(beans ...interface{}) {}

func (t *Application) Name() string    { return t.Name_ }
func (t *Application) Version() string { return constants.GetAppInfo().Version }
func (t *Application) Build() string   { return constants.GetAppInfo().Build }
func (t *Application) Profile() string { return t.Profile_ }
func (t *Application) IsDev() bool     { return t.Profile_ == "dev" }

func (t *Application) Executable() string { ex, _ := os.Executable(); return ex }

// ApplicationDir returns the data directory (a real directory). raftmod's
// snapshot factory roots its raft-snapshot folder under here when
// application.data.dir is not set explicitly.
func (t *Application) ApplicationDir() string {
	if t.DataDir_ != "" {
		return t.DataDir_
	}
	return "."
}

func (t *Application) Run(args []string) error { return nil }
func (t *Application) Active() bool             { return true }
func (t *Application) Shutdown(restart bool)    {}
func (t *Application) Restarting() bool         { return false }

// ---------------------------------------------------------------------------
// NodeService
// ---------------------------------------------------------------------------

type NodeService struct {
	Log *zap.Logger `inject:""`

	// NodeIdProp lets the operator pin a stable raft ServerID. When zero the id
	// is derived deterministically from the node name so it survives restarts.
	NodeIdProp uint64 `value:"node.id,default=0"`
	NodeName   string `value:"node.name,default=consensusdb"`
	NodeSeqNum int    `value:"node.seq,default=0"`
	DataCenter string `value:"node.dc,default=default"`

	nodeId uint64
}

func NewNodeService() *NodeService { return &NodeService{} }

func (t *NodeService) PostConstruct() error {
	if t.NodeIdProp != 0 {
		t.nodeId = t.NodeIdProp
	} else {
		h := fnv.New64a()
		host, _ := os.Hostname()
		h.Write([]byte(t.NodeName + ":" + host + ":" + strconv.Itoa(t.NodeSeqNum)))
		t.nodeId = h.Sum64()
	}
	return nil
}

func (t *NodeService) BeanName() string { return "node-service" }

func (t *NodeService) GetStats(cb func(name, value string) bool) error {
	cb("node.id", t.NodeIdHex())
	cb("node.seq", strconv.Itoa(t.NodeSeqNum))
	return nil
}

func (t *NodeService) NodeId() uint64    { return t.nodeId }
func (t *NodeService) NodeIdHex() string { return strconv.FormatUint(t.nodeId, 16) }
func (t *NodeService) NodeSeq() int      { return t.NodeSeqNum }
func (t *NodeService) DCName() string    { return t.DataCenter }

func (t *NodeService) LocalName() string { return t.nodeName() }
func (t *NodeService) LANName() string   { return t.nodeName() }
func (t *NodeService) WANName() string   { return t.nodeName() }

func (t *NodeService) nodeName() string {
	if t.NodeSeqNum == 0 {
		return t.NodeName
	}
	return t.NodeName + "-" + strconv.Itoa(t.NodeSeqNum)
}

func (t *NodeService) Issue() uuid.UUID {
	return uuid.Create(int64(t.nodeId), time.Now().UnixNano())
}

func (t *NodeService) Parse(uuid.UUID) (timestampMillis int64, nodeId int64, clock int) {
	return 0, int64(t.nodeId), 0
}

// ---------------------------------------------------------------------------
// SystemEnvironmentPropertyResolver
// ---------------------------------------------------------------------------

type EnvPropertyResolver struct{}

func NewEnvPropertyResolver() *EnvPropertyResolver { return &EnvPropertyResolver{} }

func (t *EnvPropertyResolver) BeanName() string { return "env-property-resolver" }

func (t *EnvPropertyResolver) PromptProperty(key string) (string, bool) {
	v, ok := os.LookupEnv(envKey(key))
	return v, ok
}

func (t *EnvPropertyResolver) Environ(withValues bool) []string {
	if withValues {
		return os.Environ()
	}
	return nil
}

// envKey maps a dotted property key to an environment variable name.
func envKey(key string) string {
	b := make([]byte, len(key))
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - 'a' + 'A'
		case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			b[i] = c
		default:
			b[i] = '_'
		}
	}
	return string(b)
}
