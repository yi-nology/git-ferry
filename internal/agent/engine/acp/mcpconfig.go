package acp

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/oekit/pkg/errors"
)

// acpx 的 MCP 配置文件(形状以 **acpx 0.21.0 实测源码**为准,不是照抄文档):
//
//	{
//	  "mcpServers": [
//	    {
//	      "name": "gitferry",
//	      "type": "http",                       // http | sse | stdio(缺省 stdio)
//	      "url": "http://127.0.0.1:8890/mcp",
//	      "headers": [ {"name": "X-API-Key", "value": "..."} ]   // **数组**
//	    }
//	  ]
//	}
//
// 三个容易踩的点(都与 ACP 规范同形状,但与「直觉」不同):
//   - `mcpServers` 是**数组**,不是「名字 → 配置」的对象映射
//     (acpx 的 parseMcpServers 明确 `expected array`);
//   - `headers` / `env` 是**数组** `[{name,value}]`,写成对象会「不报错但工具列表为空」;
//   - `type` 不写默认按 stdio 解析 ⇒ 写 http 端点时**必须**显式写。
//
// ## 为什么要落盘
//
// acpx 的 MCP 只能从**文件**读(`--mcp-config` / 项目配置 / 全局配置),
// 而我们自己的 `/mcp` 端点要带专用 key ⇒ key 必须进文件。
//
// 缓解措施:私有临时目录(0700)+ 文件 0600 + 跑完立刻删 + 路径不进日志。
// 这与本仓「密钥只进环境变量或 0600 文件」的约定同档。
//
// 安全性说明:acpx 会把这份配置转成 ACP 的 `session/new` 参数交给 agent
// ⇒ agent 拿到的正是它该有的那把 key(**只读角色**),与 stdio 通道同等待遇。
func writeMCPConfig(servers []MCPServer) (path string, cleanup func(), err error) {
	noop := func() {}
	if len(servers) == 0 {
		return "", noop, nil
	}
	dir, err := os.MkdirTemp("", "gitferry-mcp-")
	if err != nil {
		return "", noop, errors.Wrap(err, "创建 MCP 配置临时目录失败")
	}
	// 目录权限:os.MkdirTemp 建出来就是 **0700**(owner-only)⇒ 同机其他用户列不到。
	// 里面躺着只读 key 的专用凭据,这个权限是底线。
	cleanup = func() { _ = os.RemoveAll(dir) }

	items := make([]map[string]any, 0, len(servers))
	for i := range servers {
		s := &servers[i]
		item := map[string]any{"name": s.Name}
		switch s.Type {
		case "http", "sse":
			item["type"] = s.Type
			item["url"] = s.URL
			headers := make([]map[string]string, 0, len(s.Headers))
			for _, h := range s.Headers {
				headers = append(headers, map[string]string{"name": h.Name, "value": h.Value})
			}
			item["headers"] = headers
		default:
			item["command"] = s.Command
			item["args"] = s.Args
			env := make([]map[string]string, 0, len(s.Env))
			for _, ev := range s.Env {
				env = append(env, map[string]string{"name": ev.Name, "value": ev.Value})
			}
			item["env"] = env
		}
		items = append(items, item)
	}

	data, err := json.MarshalIndent(map[string]any{"mcpServers": items}, "", "  ")
	if err != nil {
		cleanup()
		return "", noop, errors.Wrap(err, "序列化 MCP 配置失败")
	}
	file := filepath.Join(dir, "mcp.json")
	// 0600:文件里有只读 key 的专用凭据。
	if err := os.WriteFile(file, data, 0o600); err != nil {
		cleanup()
		return "", noop, errors.Wrap(err, "写 MCP 配置文件失败")
	}
	return file, cleanup, nil
}
