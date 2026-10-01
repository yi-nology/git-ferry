// Package mcp 提供 MCP Streamable HTTP 端点，把 internal/agent/tools 注册表
// 暴露给 Claude Code / Cursor 等外部客户端。与 eino 内嵌助手共用同一工具表，避免双份漂移。
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/agent/tools"
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
			writeRPC(c, req.ID, map[string]any{"tools": listTools(ctx, reg)}, nil)
		case "tools/call":
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

func listTools(ctx context.Context, reg *tools.Registry) []map[string]any {
	names := reg.Names()
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		t := reg.ByName(n)
		if t == nil {
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
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": out}},
	}, nil
}

func writeRPC(c *app.RequestContext, id json.RawMessage, result any, rpcErr *rpcError) {
	c.SetStatusCode(http.StatusOK)
	c.JSON(http.StatusOK, jsonrpcResp{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr})
}
