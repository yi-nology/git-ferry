package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

func TestPolicy_ReadonlyDeniesDanger(t *testing.T) {
	p := NewPolicy()
	assert.Equal(t, ActionConfirm, p.Classify("run_task"))
	assert.Equal(t, ActionReadOnly, p.Classify("list_repos"))

	p.SetMode("readonly")
	assert.Equal(t, ActionDenied, p.Classify("run_task"))
	assert.Equal(t, ActionReadOnly, p.Classify("list_repos"))
}

func TestPolicy_Override(t *testing.T) {
	p := NewPolicy()
	p.Override("list_repos", ActionDenied)
	assert.Equal(t, ActionDenied, p.Classify("list_repos"))
}

func TestSetModeTool(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	reg.SetPolicy(NewPolicy())
	out, err := reg.ByName("set_permission_mode").InvokableRun(context.Background(), `{"mode":"readonly"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "readonly")
	assert.Equal(t, "readonly", reg.Policy().Mode())

	// readonly 下危险工具被拒
	ctx := WithScope(context.Background(), &fakeScope{})
	out, err = reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "denied")
}

func TestDeepAnalyze(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "T1"}}
	m.Runs = []*corebridge.SyncRun{{ID: 9, TaskKey: "t1", Status: "failed", ErrorType: "auth", ErrorMessage: "401"}}
	reg := NewRegistry(m)
	out, err := reg.ByName("deep_analyze").InvokableRun(context.Background(), `{"question":"why fail"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "playbook")
	assert.Contains(t, out, "conclusion")
}
