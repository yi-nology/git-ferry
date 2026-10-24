package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
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
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)
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

// TestRememberWithoutStore 未接记忆库时的提示。
//
// 注意:现在记忆工具走 dangerGuard,**角色闸门在「记忆库未启用」之前**
// (先判权限,再判依赖)—— 所以这里给 operator 角色。
func TestRememberWithoutStore(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleOperator), &fakeScope{})
	out, err := reg.ByName("remember").InvokableRun(ctx, `{"kind":"fact","content":"x"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "confirmation_required",
		"写记忆要走确认链路:%s", out)
}

// TestMemoryToolsRequireWritableRole 回归:记忆是**跨会话持久化的提示注入通道**
// (记忆会进每次会话的 system prompt),所以只读角色不许写/删。
func TestMemoryToolsRequireWritableRole(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	for _, tool := range []string{"remember", "forget_memory"} {
		out, err := reg.ByName(tool).InvokableRun(context.Background(), `{"kind":"fact","content":"x","id":"m1"}`)
		require.NoError(t, err)
		assert.Contains(t, out, "forbidden",
			"只读角色不得调用 %s:%s", tool, out)
	}
}
