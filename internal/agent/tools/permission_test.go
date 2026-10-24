package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

func TestPolicy_ReadonlyDeniesDanger(t *testing.T) {
	p := NewPolicy()
	assert.Equal(t, ActionConfirm, p.Classify(context.Background(), "run_task", IsDangerTool("run_task")))
	assert.Equal(t, ActionReadOnly, p.Classify(context.Background(), "list_repos", IsDangerTool("list_repos")))

	p.SetMode("readonly")
	assert.Equal(t, ActionDenied, p.Classify(context.Background(), "run_task", IsDangerTool("run_task")))
	assert.Equal(t, ActionReadOnly, p.Classify(context.Background(), "list_repos", IsDangerTool("list_repos")))
}

func TestPolicy_Override(t *testing.T) {
	p := NewPolicy()
	p.Override("list_repos", ActionDenied)
	assert.Equal(t, ActionDenied, p.Classify(context.Background(), "list_repos", IsDangerTool("list_repos")))
}

// TestPolicy_AuditModeBehavesAsDefault 审计档行为同默认档(危险工具走确认)。
func TestPolicy_AuditModeBehavesAsDefault(t *testing.T) {
	p := NewPolicy()
	p.SetMode("audit")
	assert.Equal(t, "audit", p.Mode())
	assert.Equal(t, ActionConfirm, p.Classify(context.Background(), "run_task", IsDangerTool("run_task")))
	assert.Equal(t, ActionReadOnly, p.Classify(context.Background(), "list_repos", IsDangerTool("list_repos")))
}

// TestPolicy_OverrideBeatsReadonlyMode 显式 override 优先于只读档:
// permgate 的规则集排在模式判定之下,这一档必须留在本地判定。
func TestPolicy_OverrideBeatsReadonlyMode(t *testing.T) {
	p := NewPolicy()
	p.SetMode("readonly")
	p.Override("run_task", ActionReadOnly)
	assert.Equal(t, ActionReadOnly, p.Classify(context.Background(), "run_task", IsDangerTool("run_task")))
	assert.Equal(t, ActionDenied, p.Classify(context.Background(), "rebuild_task", IsDangerTool("rebuild_task")), "未被 override 的危险工具在只读档仍被拒")
}

// TestPolicy_ReadonlyDeniesDanger_NativeUpstream 只读档对危险工具是「拒」不是「问」——
// 这条已由 oagentkit/guardx v0.14.0 原生支持(只读拦截段插在 project ask 之前、
// 以及 checkAlwaysAsk 的会话授权之后)。本测试把该口径钉死:
// 上游若改动判定顺序,这里会先红。
func TestPolicy_ReadonlyDeniesDanger_NativeUpstream(t *testing.T) {
	p := NewPolicy()
	p.SetMode("readonly")
	// 危险工具在只读档 ⇒ Deny(不是 Ask)
	for _, tool := range []string{"run_task", "test_repo_connection", "rebuild_task"} {
		assert.Equal(t, ActionDenied, p.Classify(context.Background(), tool, IsDangerTool(tool)), "只读档的 %s 应被拒", tool)
	}
	// 只读工具在只读档 ⇒ 照常放行
	assert.Equal(t, ActionReadOnly, p.Classify(context.Background(), "list_repos", IsDangerTool("list_repos")))
}

// TestSetModeTool 权限模式切换的三道闸门 + 会话级语义。
//
// 之前这个工具既没有角色闸门也没有确认,而且档位是**进程级单例**:
// 一个会话切成 readonly,会连带把**其他所有会话**的写操作都拒掉。
func TestSetModeTool(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	reg.SetPolicy(NewPolicy())

	// ① 只读角色不得拨档(没有角色 = readonly,缺省安全侧)。
	out, err := reg.ByName("set_permission_mode").InvokableRun(context.Background(), `{"mode":"readonly"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden", "只读角色不得切换权限模式:%s", out)

	// ② operator 也不够 —— 拨档是治理动作,只有 admin 能做。
	opCtx := WithScope(authz.WithRole(context.Background(), authz.RoleOperator), &fakeScope{})
	out, err = reg.ByName("set_permission_mode").InvokableRun(opCtx, `{"mode":"readonly"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden", "非 admin 不得切换权限模式:%s", out)

	// ③ admin + 会话上下文 ⇒ 先要确认(危险工具)。
	sc := &fakeScope{}
	adminCtx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)
	args := `{"mode":"readonly"}`
	out, err = reg.ByName("set_permission_mode").InvokableRun(adminCtx, args)
	require.NoError(t, err)
	assert.Contains(t, out, "confirmation_required",
		"拨档是治理动作,必须经用户确认:%s", out)

	// 模拟用户确认:标记放行后同参再调一次(真实链路就是这么走的)。
	sc.MarkConsumed("set_permission_mode", args)
	out, err = reg.ByName("set_permission_mode").InvokableRun(adminCtx, args)
	require.NoError(t, err)
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, "readonly")
	assert.Equal(t, ModeDefault, reg.Policy().Mode(),
		"档位是会话级的:进程级默认档不该被改")

	// ④ 该会话里危险工具被拒。
	out, err = reg.ByName("run_task").InvokableRun(adminCtx, `{"task_key":"t"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "denied")

	// ⑤ **另一个会话不受影响**(这正是进程级单例的老毛病)。
	otherCtx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), &fakeScope{})
	out, err = reg.ByName("run_task").InvokableRun(otherCtx, `{"task_key":"t"}`)
	require.NoError(t, err)
	assert.NotContains(t, out, "denied",
		"一个会话切只读不得连带拒掉其他会话的写操作:%s", out)
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
