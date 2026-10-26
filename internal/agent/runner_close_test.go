package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// fakeEngine 只记录「被关掉了」的引擎。
type fakeEngine struct {
	closed bool
}

func (e *fakeEngine) ID() string        { return "fake" }
func (e *fakeEngine) Label() string     { return "fake" }
func (e *fakeEngine) ModelName() string { return "fake" }
func (e *fakeEngine) Run(context.Context, engine.Session, engine.Input) (<-chan engine.Event, context.CancelFunc, error) {
	return nil, nil, nil
}
func (e *fakeEngine) Close() error {
	e.closed = true
	return nil
}

// TestRunner_Close_关掉后端 外部后端每个会话对应一个**长驻 agent 子进程**
// (原生 ACP transport)。换后端/热重建时不关旧后端,机器上就会攒孤儿进程。
func TestRunner_Close_关掉后端(t *testing.T) {
	e := &fakeEngine{}
	r := NewRunner(e, 1)
	require.NoError(t, r.Close())
	assert.True(t, e.closed, "Close 必须关掉后端")
}

// TestRunner_Close_幂等 重复 Close 不应panic(重建路径可能重复调)。
func TestRunner_Close_幂等(t *testing.T) {
	e := &fakeEngine{}
	r := NewRunner(e, 1)
	require.NoError(t, r.Close())
	require.NoError(t, r.Close())
	assert.True(t, e.closed)
}
