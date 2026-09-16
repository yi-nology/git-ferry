package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebridge "github.com/yi-nology/git-sync-service/internal/corebridge"

	"github.com/yi-nology/git-sync-service/internal/agent/tools/toolstest"
)

type fakeScope struct {
	consumedTool string
	consumedArgs string
	lastTool     string
	lastToken    string
	lastArgs     string
	pendingSet   bool
}

func (f *fakeScope) SetPending(toolName, argsJSON string) (string, error) {
	f.pendingSet = true
	return "tok123", nil
}
func (f *fakeScope) HasConsumed(toolName, argsJSON string) bool {
	return f.consumedTool == toolName && f.consumedArgs == argsJSON
}
func (f *fakeScope) MarkConsumed(toolName, argsJSON string) {
	f.consumedTool, f.consumedArgs = toolName, argsJSON
}
func (f *fakeScope) SetLastConfirm(toolName, token, argsJSON string) {
	f.lastTool, f.lastToken, f.lastArgs = toolName, token, argsJSON
}
func (f *fakeScope) LastConfirm() (string, string, string) {
	return f.lastTool, f.lastToken, f.lastArgs
}

func TestRegistry_AllToolsRegistered(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	// 13 个工具:7 只读 + 3 概览类 + 3 危险
	assert.Len(t, reg.Names(), 13)
	for _, name := range []string{
		"list_repos", "get_repo", "list_branches", "list_tasks", "get_task",
		"list_sync_history", "get_run_detail", "list_platforms", "list_webhook_rules",
		"get_system_overview", "run_task", "test_repo_connection", "test_platform_connection",
	} {
		assert.NotNil(t, reg.ByName(name), "缺少工具 %s", name)
	}
}

func TestListRepos(t *testing.T) {
	m := toolstest.NewMock()
	m.Repos = []*corebridge.Repo{
		{Key: "demo", Name: "demo", Platform: "github", Status: "active", CloneURL: "https://github.com/o/demo.git"},
	}
	m.RepoTotal = 1
	reg := NewRegistry(m)

	out, err := reg.ByName("list_repos").InvokableRun(context.Background(), `{"page":1}`)
	require.NoError(t, err)
	assert.Contains(t, out, "demo")
	assert.Nil(t, m.LastFilter, "无关键字时 filter 应为 nil")
}

func TestListRepos_WithKeyword(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)

	_, err := reg.ByName("list_repos").InvokableRun(context.Background(), `{"keyword":"demo"}`)
	require.NoError(t, err)
	require.NotNil(t, m.LastFilter)
	assert.Equal(t, "demo", m.LastFilter.Search)
}

func TestGetRepo_NotFound(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	out, err := reg.ByName("get_repo").InvokableRun(context.Background(), `{"key":"x"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "未找到")
}

func TestGetRunDetail_ScansHistory(t *testing.T) {
	m := toolstest.NewMock()
	m.Runs = []*corebridge.SyncRun{
		{ID: 7, Status: "failed", ErrorMessage: "timeout", Details: "Step 1: Fetch...\nStep 2: Push failed"},
	}
	m.RunTotal = 1
	reg := NewRegistry(m)

	out, err := reg.ByName("get_run_detail").InvokableRun(context.Background(), `{"task_key":"t1","run_id":7}`)
	require.NoError(t, err)
	assert.Contains(t, out, "timeout")
	assert.Contains(t, out, "Push failed")
}

func TestGetRunDetail_MissingRun(t *testing.T) {
	m := toolstest.NewMock()
	m.Runs = []*corebridge.SyncRun{{ID: 1}}
	reg := NewRegistry(m)

	out, err := reg.ByName("get_run_detail").InvokableRun(context.Background(), `{"task_key":"t1","run_id":99}`)
	require.NoError(t, err)
	assert.Contains(t, out, "未找到")
}

func TestRunTask_RequiresConfirmation(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)
	sc := &fakeScope{}
	ctx := WithScope(context.Background(), sc)

	out, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"confirmation_required"`)
	assert.Empty(t, m.LastRunTaskKey, "未确认前不得执行")
	assert.True(t, sc.pendingSet)
	assert.Equal(t, "run_task", sc.lastTool)
	assert.Equal(t, "tok123", sc.lastToken)
}

func TestRunTask_ExecutesAfterConfirmation(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)
	sc := &fakeScope{}
	ctx := WithScope(context.Background(), sc)

	// 第一次:发起确认
	_, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)

	// handler 确认后放行
	sc.MarkConsumed("run_task", `{"task_key":"t1"}`)
	out, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"ok"`)
	assert.Equal(t, "t1", m.LastRunTaskKey)
}

func TestRunTask_WithoutScope_Refused(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	out, err := reg.ByName("run_task").InvokableRun(context.Background(), `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "会话上下文缺失")
}
