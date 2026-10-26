package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebridge "github.com/yi-nology/git-ferry/internal/corebridge"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

type fakeScope struct {
	consumedTool string
	consumedArgs string
	lastTool     string
	lastToken    string
	lastArgs     string
	pendingSet   bool
	permMode     string
}

func (f *fakeScope) SetPending(toolName, argsJSON string) (string, error) {
	f.pendingSet = true
	return "tok123", nil
}
func (f *fakeScope) ConsumeConsumed(toolName, argsJSON string) bool {
	if f.consumedTool == toolName && f.consumedArgs == argsJSON {
		f.consumedTool, f.consumedArgs = "", ""
		return true
	}
	return false
}
func (f *fakeScope) MarkConsumed(toolName, argsJSON string) {
	f.consumedTool, f.consumedArgs = toolName, argsJSON
}
func (f *fakeScope) SetPermissionMode(mode string) bool {
	f.permMode = mode
	return true
}

func (f *fakeScope) PermissionMode() string { return f.permMode }

func (f *fakeScope) SetLastConfirm(toolName, token, argsJSON string) {
	f.lastTool, f.lastToken, f.lastArgs = toolName, token, argsJSON
}
func (f *fakeScope) LastConfirm() (string, string, string) {
	return f.lastTool, f.lastToken, f.lastArgs
}

func TestRegistry_AllToolsRegistered(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	// 42 个工具:只读查询 + 概览/治理/记忆 + 危险(含 run_dr_drill) + plan/diagnose + ops_todo
	// + 元数据快照 + 历史同步十件套(通道列表/计划/解法/执行记录/链校验/预检/裁决列表/写裁决/一次同步/凭据轮换)
	// + 委派外部 coding agent(ai_code_delegate,危险)
	assert.Len(t, reg.Names(), 42)
	for _, name := range []string{
		"list_repos", "get_repo", "list_branches", "list_tasks", "get_task",
		"list_sync_history", "get_run_detail", "list_platforms", "list_webhook_rules",
		"get_system_overview", "get_ops_todo", "run_task", "test_repo_connection", "test_platform_connection",
		"get_rpo_report", "get_backup_integrity", "get_drift_report", "get_audit_chain", "run_dr_drill",
		"get_metadata_snapshots", "ai_code_delegate",
		"list_histsync_channels", "plan_histsync_channel", "resolve_histsync_channel",
		"list_histsync_runs", "verify_histsync_channel_audit",
		"list_histsync_overrides", "set_histsync_override", "preview_histsync_channel",
		"sync_histsync_channel", "rotate_history_target_credential",
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
	require.NotNil(t, m.LastFilter, "始终传非 nil filter(core 无 nil 防护)")
	assert.Empty(t, m.LastFilter.Search)
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
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	out, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"confirmation_required"`)
	assert.Empty(t, m.RunTaskKey(), "未确认前不得执行")
	assert.True(t, sc.pendingSet)
	assert.Equal(t, "run_task", sc.lastTool)
	assert.Equal(t, "tok123", sc.lastToken)
}

func TestRunTask_ExecutesAfterConfirmation(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	// 第一次:发起确认
	_, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)

	// handler 确认后放行
	sc.MarkConsumed("run_task", `{"task_key":"t1"}`)
	out, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"ok"`)
	assert.Equal(t, "t1", m.RunTaskKey())
}

func TestRunTask_WithoutScope_Refused(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	// 角色给足:这里要验的是「没有会话 scope → 拒绝」这条路径,
	// 不是「角色不足 → 拒绝」。
	out, err := reg.ByName("run_task").InvokableRun(
		authz.WithRole(context.Background(), authz.RoleAdmin), `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "没有会话上下文")
	assert.Contains(t, out, "CLI", "错误信息要说明「怎么办」")
}

// TestDangerToolRefusedForReadonlyRole 只读角色不能执行危险工具。
//
// 之前危险工具只是「因为没注入会话 scope」才被挡住的 ——
// 那是偶然,不是设计:一旦注入了 scope,只读用户就能触发推送/写裁决。
func TestDangerToolRefusedForReadonlyRole(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleReadonly), &fakeScope{})
	out, err := reg.ByName("run_task").InvokableRun(ctx, `{"task_key":"t"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden")
}

func TestErrJSON_EscapesSpecialChars(t *testing.T) {
	// err 内含引号/换行时不得破坏 JSON 结构
	out := errJSON("查询失败", assertErr(`boom "quoted" \n`))
	require.True(t, json.Valid([]byte(out)), "errJSON must produce valid JSON, got: %s", out)
	assert.Contains(t, out, "查询失败")
	assert.Contains(t, out, `quoted`)
}

func TestMarshalJSON_FallbackOnFailure(t *testing.T) {
	// channel 不可序列化,应回退到错误负载而非 panic/空串
	out := make(chan int)
	got := marshalJSON(out)
	require.True(t, json.Valid([]byte(got)), "marshalJSON fallback must be valid JSON")
	assert.Contains(t, got, "error")
}

// assertErr 构造带特殊字符的 error。
type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestGetRPOReport(t *testing.T) {
	m := toolstest.NewMock()
	m.RPO = &corebridge.RPOReport{OverallRPOHuman: "2h", WorstTask: "t1"}
	reg := NewRegistry(m)
	out, err := reg.ByName("get_rpo_report").InvokableRun(context.Background(), `{"max_seconds":3600}`)
	require.NoError(t, err)
	assert.Contains(t, out, "2h")
}

func TestGetDriftReport(t *testing.T) {
	m := toolstest.NewMock()
	m.Drift = &corebridge.DriftReport{Checked: 2, Drifted: 1, Items: []corebridge.DriftItem{
		{TaskKey: "t1", Drifted: true, Message: "diverged"},
	}}
	reg := NewRegistry(m)
	out, err := reg.ByName("get_drift_report").InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.Contains(t, out, "diverged")
}

func TestRunDRDrill_RequiresConfirm(t *testing.T) {
	m := toolstest.NewMock()
	reg := NewRegistry(m)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)
	out, err := reg.ByName("run_dr_drill").InvokableRun(ctx, `{"name":"a.bundle"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "confirmation_required")
	assert.Equal(t, "run_dr_drill", sc.lastTool)
}

// TestSyncHistoryForcePushRequiresAdmin 回归:强制推送要 admin 角色。
//
// 之前「强制推送要 admin」只在 HTTP handler 里判 —— MCP / AI 助手两条路没判,
// operator 就能强推覆盖掉远端别人的提交。
// 现在闸门有两层:工具层判角色,core 层要求「有批准者」。
func TestSyncHistoryForcePushRequiresAdmin(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())

	// operator + 完整 scope → 仍应被拒(拒绝来自角色判定)。
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleOperator), &fakeScope{})
	out, err := reg.ByName("sync_histsync_channel").InvokableRun(ctx, `{"channel_id":1,"force_push":true}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden")
	assert.Contains(t, out, "admin")

	// admin → 不被角色判定拦下,进入确认流程。
	ctx = WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), &fakeScope{})
	out, err = reg.ByName("sync_histsync_channel").InvokableRun(ctx, `{"channel_id":1,"force_push":true}`)
	require.NoError(t, err)
	assert.NotContains(t, out, "forbidden")
	assert.Contains(t, out, "confirmation_required")
}

// TestHistsyncToolSchemasAreClean 只读工具的 schema 里不该出现写参数。
//
// 入参 schema 是模型看到的**唯一契约**:带了模型没用上的参数,模型会以为它有用
// (实际被静默忽略);而带了必填的写参数,这个只读工具对模型直接不可用
// (schema 校验就失败)。
//
// 这条测试是防回归的:以前所有工具共用同一套入参结构,污染就是这么进来的。
func TestHistsyncToolSchemasAreClean(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	readOnly := []string{
		"list_histsync_channels", "plan_histsync_channel", "preview_histsync_channel",
		"list_histsync_runs", "verify_histsync_channel_audit", "list_histsync_overrides",
	}
	for _, name := range readOnly {
		tl := reg.ByName(name)
		require.NotNil(t, tl, "工具 %s 应存在", name)
		info, err := tl.Info(context.Background())
		require.NoError(t, err)
		raw, err := json.Marshal(info)
		require.NoError(t, err)
		schema := string(raw)
		assert.NotContains(t, schema, "force_push",
			"%s 是只读工具,schema 里不该有 force_push", name)
		assert.NotContains(t, schema, "verdict",
			"%s 是只读工具,schema 里不该有写裁决的参数", name)
		assert.NotContains(t, schema, "decided_by",
			"%s 是只读工具,schema 里不该有写裁决的参数", name)
	}

	// sync 是唯一带 force_push 的。
	tl := reg.ByName("sync_histsync_channel")
	require.NotNil(t, tl)
	info, err := tl.Info(context.Background())
	require.NoError(t, err)
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "force_push")
}
