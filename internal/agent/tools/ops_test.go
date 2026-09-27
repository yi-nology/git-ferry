package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

func TestGetSyncHealth_ReturnsLevels(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{
		{Key: "t1", Name: "Good", Cron: "0 * * * *"},
		{Key: "t2", Name: "Bad"},
	}
	// Mock.ListHistory 对所有 task 返回同一列表,这里用单条 success 覆盖
	m.Runs = []*corebridge.SyncRun{{ID: 1, TaskKey: "t1", Status: "success"}}

	reg := NewRegistry(m)
	out, err := reg.ByName("get_sync_health").InvokableRun(context.Background(), `{"limit":10}`)
	require.NoError(t, err)
	var body struct {
		Items []struct {
			Key    string   `json:"key"`
			Score  int      `json:"score"`
			Level  string   `json:"level"`
			Issues []string `json:"issues"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body))
	require.NotEmpty(t, body.Items)
	assert.Equal(t, "gold", body.Items[0].Level)
}

func TestGetRepoInventory_Orphans(t *testing.T) {
	m := toolstest.NewMock()
	m.Repos = []*corebridge.Repo{
		{Key: "r1", Name: "Covered"},
		{Key: "r2", Name: "Orphan"},
	}
	m.Tasks = []*corebridge.SyncTask{
		{Key: "t1", SourceRepoKey: "r1", TargetRepoKey: "r1x"},
	}
	reg := NewRegistry(m)
	out, err := reg.ByName("get_repo_inventory").InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"orphan_count":1`)
	assert.Contains(t, out, `"key":"r2"`)
}

func TestRetrySyncRun_RequiresConfirmFirst(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)
	sc := &fakeScope{}
	ctx := WithScope(context.Background(), sc)
	out, err := reg.ByName("retry_sync_run").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "confirmation_required")
	assert.Empty(t, m.RunTaskKey())

	sc.MarkConsumed("retry_sync_run", `{"task_key":"t1"}`)
	out, err = reg.ByName("retry_sync_run").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"ok"`)
	assert.Equal(t, "t1", m.RunTaskKey())
}

func TestPlanMode_ReturnsSteps(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	out, err := reg.ByName("plan_mode").InvokableRun(context.Background(), `{"goal":"修复 t1 反复失败"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "plan")
	assert.Contains(t, out, "need_confirm")
}

func TestRememberWithoutStore(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	out, err := reg.ByName("remember").InvokableRun(context.Background(), `{"kind":"fact","content":"x"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "记忆库未启用")
}
