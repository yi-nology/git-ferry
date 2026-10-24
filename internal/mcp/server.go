// Package mcp 提供 MCP Streamable HTTP 端点，把 internal/agent/tools 注册表
// 暴露给 Claude Code / Cursor 等外部客户端。与 eino 内嵌助手共用同一工具表，避免双份漂移。
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	gitsync "github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/pkg/authz"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

const protocolVersion = "2024-11-05"

type jsonrpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handler POST /mcp — JSON-RPC（initialize / tools/list / tools/call / ping）。
func Handler(reg *tools.Registry) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 角色**跟着 ctx 往后走**:工具层要判角色
		// (危险工具、强制推送都需要知道调用者是谁)。
		ctx = authz.WithRole(ctx, gitsync.GetAuthRole(c))
		// **确认由客户端负责**:MCP 客户端在把工具调用发过来之前,
		// 自己就已经向用户展示并征求过同意了;MCP 又是无状态的,
		// 服务端这边「登记待确认」的令牌无处可去,危险工具会直接不可用。
		//
		// 注意:这只放行**确认**这一步 —— 角色闸门与强制推送的 admin 闸门
		// 都照旧生效(见 tools.ClientConfirmedScope 的说明)。
		ctx = tools.WithClientConfirmedScope(ctx)
		var req jsonrpcReq
		body, berr := c.Body()
		if berr != nil {
			writeRPC(c, nil, nil, &rpcError{Code: -32700, Message: "read body: " + berr.Error()})
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeRPC(c, nil, nil, &rpcError{Code: -32700, Message: "parse error: " + err.Error()})
			return
		}
		switch req.Method {
		case "initialize":
			writeRPC(c, req.ID, map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "gitferry", "version": "1.0.0"},
			}, nil)
		case "notifications/initialized", "notifications/cancelled":
			c.SetStatusCode(http.StatusAccepted)
		case "tools/list":
			writeRPC(c, req.ID, map[string]any{"tools": listTools(ctx, reg, gitsync.GetAuthRole(c))}, nil)
		case "tools/call":
			// **危险工具要写角色**。
			//
			// MCP 端点只有鉴权、没有路由层的 RBAC —— 工具是直接调 core
			// service 的,绕开了 HTTP 路由的 WriteGuard/AdminGuard。
			// 危险工具(会改对外状态:推送到远端、写裁决、轮换凭据)
			// 对只读角色必须关掉,否则 OIDC 的默认 readonly 用户
			// 确认一下就能把内容推到远端。
			//
			// 「是否危险」由注册表派生(reg.IsDanger)—— 事实源在工具注册处。
			if name := mcpToolName(req.Params); name != "" && reg.IsDanger(name) {
				if role := gitsync.GetAuthRole(c); !gitsync.CanWriteRole(role) {
					writeRPC(c, req.ID, nil, &rpcError{
						Code:    -32603,
						Message: "forbidden: role " + string(role) + " cannot run " + name,
					})
					return
				}
			}
			result, rpcErr := callTool(ctx, reg, req.Params)
			writeRPC(c, req.ID, result, rpcErr)
		case "ping":
			writeRPC(c, req.ID, map[string]any{}, nil)
		default:
			writeRPC(c, req.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method})
		}
	}
}

// Status GET /mcp — 能力探测（CLI/连通性检查）。
func Status(reg *tools.Registry) app.HandlerFunc {
	return func(_ context.Context, c *app.RequestContext) {
		response.Success(c, map[string]any{
			"protocol": protocolVersion,
			"tools":    len(reg.Names()),
			"names":    reg.Names(),
		})
	}
}

// listTools 列出工具清单。
//
// **按角色裁剪**:只读角色不该看见写工具 ——
// 看得见、点一下却被拒,比看不见更让人困惑。
func listTools(ctx context.Context, reg *tools.Registry, role gitsync.Role) []map[string]any {
	names := reg.Names()
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		t := reg.ByName(n)
		if t == nil {
			continue
		}
		if reg.IsDanger(n) && !gitsync.CanWriteRole(role) {
			continue
		}
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		desc := n
		if info, err := t.Info(ctx); err == nil && info != nil {
			if info.Name != "" {
				n = info.Name
			}
			if info.Desc != "" {
				desc = info.Desc
			}
			// ToolInfo.MarshalJSON 会带出 params / json_schema
			if raw, merr := json.Marshal(info); merr == nil {
				var blob map[string]any
				if json.Unmarshal(raw, &blob) == nil {
					if js, ok := blob["json_schema"].(map[string]any); ok && len(js) > 0 {
						schema = js
					} else if params, ok := blob["params"].(map[string]any); ok && len(params) > 0 {
						props := map[string]any{}
						for k, v := range params {
							props[k] = v
						}
						schema = map[string]any{"type": "object", "properties": props}
					}
				}
			}
		}
		out = append(out, map[string]any{
			"name":        n,
			"description": desc,
			"inputSchema": schema,
		})
	}
	return out
}

// mcpToolName 从 tools/call 参数里取工具名(取不到返回空串)。
func mcpToolName(params json.RawMessage) string {
	var p struct {
		Name string `json:"name"`
	}
	if len(params) == 0 {
		return ""
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}
	return p.Name
}

func callTool(ctx context.Context, reg *tools.Registry, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
	}
	if p.Name == "" {
		return nil, &rpcError{Code: -32602, Message: "missing tool name"}
	}
	t := reg.ByName(p.Name)
	if t == nil {
		return nil, &rpcError{Code: -32601, Message: "unknown tool: " + p.Name}
	}
	args := string(p.Arguments)
	if args == "" {
		args = "{}"
	}
	out, err := t.InvokableRun(ctx, args)
	if err != nil {
		// 工具执行错误作为 tool result（isError）返回，便于模型自行纠正
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	// **同时给出结构化结果**。
	//
	// 工具内部返回的本来就是结构化 JSON(marshalJSON / map[string]any),
	// 但 MCP 边界上只包成了文本块 ⇒ 客户端(Claude Code / Cursor)只能
	// 自己去解析字符串。`structuredContent` 是 MCP 规范里与 content 并列的字段,
	// 老客户端不认识会忽略(行为退化成现状,无破坏)。
	//
	// 保留 content 文本块是为了兼容:不是所有客户端都读 structuredContent。
	result := map[string]any{
		"content": []map[string]any{{"type": "text", "text": out}},
	}
	if structured, ok := decodeJSONObject(out); ok {
		result["structuredContent"] = structured
	}
	return result, nil
}

// decodeJSONObject 尝试把工具输出解析成 JSON 对象。
//
// 只有**对象**才给 structuredContent:数组/标量在 MCP 规范里
// 要求 structuredContent 是 object,给错类型比不给更糟。
func decodeJSONObject(out string) (any, bool) {
	trimmed := strings.TrimSpace(out)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return nil, false
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, false
	}
	return v, true
}

func writeRPC(c *app.RequestContext, id json.RawMessage, result any, rpcErr *rpcError) {
	c.SetStatusCode(http.StatusOK)
	c.JSON(http.StatusOK, jsonrpcResp{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr})
}
