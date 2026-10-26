package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitsync "github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
)

// doRPCAs 以指定角色调用 MCP。
func doRPCAs(t *testing.T, reg *tools.Registry, role gitsync.Role, payload string) jsonrpcResp {
	t.Helper()
	body := app.NewContext(0)
	body.Request.SetBody([]byte(payload))
	body.Request.Header.SetMethod(consts.MethodPost)
	gitsync.SetAuthRole(body, role)
	Handler(reg)(context.Background(), body)
	var resp jsonrpcResp
	if err := json.Unmarshal(body.Response.Body(), &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", body.Response.Body(), err)
	}
	return resp
}

// TestMCPDangerToolRequiresWriteRole 回归:MCP 端点的危险工具要写角色。
//
// MCP 端点只有鉴权、没有路由层的 RBAC —— 工具直接调 core service,
// 绕开了 HTTP 路由的 WriteGuard。不在这里补一道,OIDC 的默认 readonly
// 用户确认一下就能把内容推到远端。
func TestMCPDangerToolRequiresWriteRole(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())

	for _, tc := range []struct {
		role   gitsync.Role
		tool   string
		denied bool
	}{
		{gitsync.RoleReadonly, "sync_histsync_channel", true},
		{gitsync.RoleReadonly, "set_histsync_override", true},
		{gitsync.RoleReadonly, "rotate_history_target_credential", true},
		{gitsync.RoleReadonly, "list_histsync_channels", false},
		// 记忆是**跨会话持久化的提示注入通道**(记忆会进每次会话的
		// system prompt)⇒ 只读角色不许写/删。
		{gitsync.RoleReadonly, "remember", true},
		{gitsync.RoleReadonly, "forget_memory", true},
		// 拨权限档是治理动作 ⇒ 只读角色不许拨。
		{gitsync.RoleReadonly, "set_permission_mode", true},
		{gitsync.RoleOperator, "sync_histsync_channel", false},
		{gitsync.RoleAdmin, "sync_histsync_channel", false},
	} {
		t.Run(string(tc.role)+"/"+tc.tool, func(t *testing.T) {
			payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` +
				tc.tool + `","arguments":{}}}`
			resp := doRPCAs(t, reg, tc.role, payload)
			if tc.denied {
				if resp.Error == nil || !strings.Contains(resp.Error.Message, "forbidden") {
					t.Fatalf("只读角色不应能执行危险工具 %s: %+v", tc.tool, resp.Error)
				}
				return
			}
			if resp.Error != nil && strings.Contains(resp.Error.Message, "forbidden") {
				t.Fatalf("角色 %s 不应被拒: %s", tc.role, resp.Error.Message)
			}
		})
	}
}

// TestMCPToolsListFilteredByRole 只读角色的工具清单里不该出现写工具。
//
// 看得见、点一下却被拒,比看不见更让人困惑。
func TestMCPToolsListFilteredByRole(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())

	ro := doRPCAs(t, reg, gitsync.RoleReadonly, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if ro.Error != nil {
		t.Fatal(ro.Error)
	}
	raw, _ := json.Marshal(ro.Result)
	list := string(raw)
	assert.NotContains(t, list, "sync_histsync_channel", "只读角色不该看见写工具")
	assert.Contains(t, list, "list_histsync_channels", "只读角色应当能看见只读工具")

	admin := doRPCAs(t, reg, gitsync.RoleAdmin, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if admin.Error != nil {
		t.Fatal(admin.Error)
	}
	rawAdmin, _ := json.Marshal(admin.Result)
	assert.Contains(t, string(rawAdmin), "sync_histsync_channel", "admin 应当能看见写工具")
}

// TestMCPSyncForcePushRequiresAdmin 回归:强制推送经 MCP 也必须 admin。
//
// 之前「强制推送要 admin」只在 HTTP handler 里判,MCP 这条路只判了
// 「能不能写」→ operator 就能强推覆盖掉远端别人的提交。
// 现在闸门下沉到 core(没有批准者就不许强推),三条入口口径一致。
func TestMCPSyncForcePushRequiresAdmin(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())

	// 强制推送的 admin 闸门在 MCP 上同样生效。
	for _, tc := range []struct {
		role   gitsync.Role
		denied bool
	}{
		{gitsync.RoleReadonly, true},
		{gitsync.RoleOperator, true},
		{gitsync.RoleAdmin, false},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` +
				`{"name":"sync_histsync_channel","arguments":{"channel_id":1,"force_push":true}}}`
			resp := doRPCAs(t, reg, tc.role, payload)
			body := respBody(t, resp)
			if tc.denied {
				assert.Contains(t, body, "forbidden",
					"角色 %s 不应能经 MCP 强制推送", tc.role)
				return
			}
			assert.NotContains(t, body, "forbidden",
				"admin 应能经 MCP 强制推送")
		})
	}
}

// TestMCPWriteToolsUsable 回归:MCP 的写工具必须**可用**。
//
// 曾经 MCP 端点从不注入会话 scope → 危险工具一律「会话上下文缺失」被拒
// → Claude Code / Cursor 根本调不了历史同步的写操作。
//
// 现在确认由客户端负责(见 tools.ClientConfirmedScope),服务端保留角色闸门。
func TestMCPWriteToolsUsable(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())

	for _, name := range []string{
		"sync_histsync_channel", "set_histsync_override", "rotate_history_target_credential",
	} {
		t.Run(name, func(t *testing.T) {
			payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` +
				`{"name":"` + name + `","arguments":{"channel_id":1,"target_id":1,"path":"p","verdict":"keep_target"}}}`
			resp := doRPCAs(t, reg, gitsync.RoleOperator, payload)
			body := respBody(t, resp)
			assert.NotContains(t, body, "会话上下文缺失",
				"%s 在 MCP 上不该因缺少会话 scope 被拒", name)
			assert.NotContains(t, body, "forbidden",
				"operator 有写权限,%s 不该被角色闸门拦下", name)
		})
	}
}

// respBody 取响应的可读文本:工具结果或 RPC 错误都算。
//
// 危险工具被角色闸门拦下时,服务端走的是 **RPC error** 分支
// (Result 为 null)—— 只看 Result 会把错误当成「什么都没返回」。
func respBody(t *testing.T, resp jsonrpcResp) string {
	t.Helper()
	if resp.Error != nil {
		return resp.Error.Message
	}
	raw, err := json.Marshal(resp.Result)
	require.NoError(t, err)
	return string(raw)
}

// mustJSON 把结果序列化(便于断言文本)。

// TestMCPToolResultCarriesStructuredContent 工具结果要**同时**给出结构化内容。
//
// 工具内部返回的本来就是结构化 JSON,但 MCP 边界上只包成了文本块 ⇒
// 客户端(Claude Code / Cursor)只能自己去解析字符串。
// `structuredContent` 是 MCP 规范里与 content 并列的字段;
// 老客户端不认识会忽略(退化成现状,无破坏),新客户端能直接消费类型化结果。
func TestMCPToolResultCarriesStructuredContent(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())

	payload := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`
	resp := doRPCAs(t, reg, gitsync.RoleReadonly, payload)
	require.Nil(t, resp.Error)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok, "结果应是对象:%+v", resp.Result)

	structured, ok := result["structuredContent"].(map[string]any)
	require.True(t, ok, "工具结果必须带 structuredContent:%+v", result)
	assert.Contains(t, structured, "repos")

	// 文本块仍然保留(兼容不读 structuredContent 的客户端)。
	content, ok := result["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	block, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, _ := block["text"].(string)
	assert.Contains(t, text, "\"repos\"")
}

// TestMCPHistsyncToolSchemas 钉住 histsync 工具的**入参形状**。
//
// 入参 schema 是模型看到的唯一契约,形状错了模型就调不通:
//   - `list_histsync_channels` **不该要求 channel_id** —— 列全部通道用不上它。
//     之前它复用 `historyChannelRef`,而那个结构的 channel_id 没有 omitempty
//     ⇒ schema 把它标成**必填** ⇒ 模型只能被迫随便填一个 id。
//   - 只读工具**不该带写参数**(force_push 之类):给了模型会以为它有用,
//     实际被静默忽略。
func TestMCPHistsyncToolSchemas(t *testing.T) {
	reg := tools.NewRegistry(toolstest.NewMock())
	resp := doRPCAs(t, reg, gitsync.RoleAdmin, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Nil(t, resp.Error)

	raw, err := json.Marshal(resp.Result)
	require.NoError(t, err)
	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Required   []string       `json:"required"`
				Properties map[string]any `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(raw, &list))

	byName := map[string]int{}
	for i, tl := range list.Tools {
		byName[tl.Name] = i
	}

	cases := []struct {
		tool     string
		required []string
		absent   []string
	}{
		{"list_histsync_channels", nil, []string{"channel_id"}},
		{"list_histsync_runs", []string{"channel_id"}, nil},
		{"plan_histsync_channel", []string{"channel_id"}, nil},
		{"resolve_histsync_channel", []string{"channel_id"}, nil},
		{"list_histsync_overrides", []string{"channel_id"}, nil},
		{"verify_histsync_channel_audit", []string{"channel_id"}, nil},
		{"preview_histsync_channel", []string{"channel_id"}, []string{"force_push"}},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			idx, ok := byName[c.tool]
			require.True(t, ok, "工具清单里应当有 %s", c.tool)
			schema := list.Tools[idx].InputSchema
			for _, r := range c.required {
				assert.Contains(t, schema.Required, r,
					"%s 应当要求 %s", c.tool, r)
			}
			for _, a := range c.absent {
				assert.NotContains(t, schema.Properties, a,
					"%s 不该有 %s 入参", c.tool, a)
			}
		})
	}
}
