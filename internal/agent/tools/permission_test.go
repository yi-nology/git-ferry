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

// TestPolicy_AuditModeBehavesAsDefault 审计档行为同默认档(危险工具走确认)。
func TestPolicy_AuditModeBehavesAsDefault(t *testing.T) {
	p := NewPolicy()
	p.SetMode("audit")
	assert.Equal(t, "audit", p.Mode())
	assert.Equal(t, ActionConfirm, p.Classify("run_task"))
	assert.Equal(t, ActionReadOnly, p.Classify("list_repos"))
}

// TestPolicy_OverrideBeatsReadonlyMode 显式 override 优先于只读档:
// permgate 的规则集排在模式判定之下,这一档必须留在本地判定。
func TestPolicy_OverrideBeatsReadonlyMode(t *testing.T) {
	p := NewPolicy()
	p.SetMode("readonly")
	p.Override("run_task", ActionReadOnly)
	assert.Equal(t, ActionReadOnly, p.Classify("run_task"))
	assert.Equal(t, ActionDenied, p.Classify("rebuild_task"), "未被 override 的危险工具在只读档仍被拒")
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
		assert.Equal(t, ActionDenied, p.Classify(tool), "只读档的 %s 应被拒", tool)
	}
	// 只读工具在只读档 ⇒ 照常放行
	assert.Equal(t, ActionReadOnly, p.Classify("list_repos"))
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
