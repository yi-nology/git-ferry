// apidoc 解析 idl/*.thrift 生成 docs/openapi.json(OpenAPI 3.0),
// 服务端经 internal/pkg/swagger 内嵌 Swagger UI 消费该文件。
//
//	go run ./cmd/apidoc          # 生成 docs/openapi.json
//	make apidoc                  # 同上(Makefile target)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- Thrift IDL 解析 ----------

type thriftField struct {
	ID       int
	Name     string
	Type     string
	JSONName string // api.json 注解,缺省同 Name
	Query    string // api.query 注解
	Required bool
}

type thriftStruct struct {
	Name   string
	Fields []thriftField
}

type thriftMethod struct {
	Name       string
	ReturnType string
	ReqType    string
	HTTPMethod string // GET / POST
	Path       string
}

type thriftFile struct {
	Structs  map[string]*thriftStruct
	Services map[string][]thriftMethod
}

// parseThriftFile 用逐行解析提取 struct 和 service 定义。
// 覆盖项目 IDL 的全部语法:include、struct 基础类型/list/map、
// api.json/api.query/api.get/api.post 注解。
func parseThriftFile(path string) (*thriftFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 路径来自命令行参数,由部署方控制
	if err != nil {
		return nil, err
	}
	tf := &thriftFile{
		Structs:  make(map[string]*thriftStruct),
		Services: make(map[string][]thriftMethod),
	}

	var (
		inStruct  bool
		curStruct *thriftStruct
		inService bool
		curSvc    string
	)

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		// include 行跳过,字段类型已带模块前缀无需展开
		if strings.HasPrefix(line, "include ") {
			continue
		}

		// struct 定义开始
		if strings.HasPrefix(line, "struct ") {
			name := strings.Fields(strings.TrimPrefix(line, "struct "))[0]
			name = strings.TrimSuffix(name, "{")
			curStruct = &thriftStruct{Name: name}
			inStruct = true
			continue
		}

		// struct 结束
		if inStruct && strings.HasPrefix(line, "}") {
			tf.Structs[curStruct.Name] = curStruct
			inStruct = false
			curStruct = nil
			continue
		}

		// struct 字段
		if inStruct {
			if f := parseField(line); f != nil {
				curStruct.Fields = append(curStruct.Fields, *f)
			}
			continue
		}

		// service 定义
		if strings.HasPrefix(line, "service ") {
			svcName := strings.Fields(strings.TrimPrefix(line, "service "))[0]
			svcName = strings.TrimSuffix(svcName, "{")
			curSvc = svcName
			inService = true
			continue
		}

		if inService && strings.HasPrefix(line, "}") {
			inService = false
			curSvc = ""
			continue
		}

		// service 方法
		if inService && curSvc != "" {
			if m := parseMethod(line); m != nil {
				tf.Services[curSvc] = append(tf.Services[curSvc], *m)
			}
		}
	}
	return tf, nil
}

// parseField 解析 struct 字段行,如:
//
//	1: string key (api.query="key")
//	2: list<repo.RepoInfo> list (api.json="list")
func parseField(line string) *thriftField {
	// 格式: ID: type name (annotations) [,]?
	// 简化: 找到第一个 ':' 后解析
	idx := strings.Index(line, ":")
	if idx < 0 {
		return nil
	}
	idStr := strings.TrimSpace(line[:idx])
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		return nil
	}
	rest := strings.TrimSpace(line[idx+1:])

	// 分离注解部分
	var annotations string
	if annoStart := strings.Index(rest, "("); annoStart >= 0 {
		annotations = rest[annoStart:]
		rest = strings.TrimSpace(rest[:annoStart])
	}
	rest = strings.TrimSuffix(rest, ",")
	rest = strings.TrimSpace(rest)

	parts := strings.Fields(rest)
	if len(parts) < 2 {
		return nil
	}
	typ := parts[0]
	name := parts[1]
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}

	f := &thriftField{
		ID:       id,
		Name:     name,
		Type:     typ,
		JSONName: name, // 默认同字段名
	}

	// 解析 api.json 注解
	if annotations != "" {
		if jsonName := extractAnnotation(annotations, "api.json"); jsonName != "" {
			f.JSONName = jsonName
		}
		f.Query = extractAnnotation(annotations, "api.query")
	}

	return f
}

// parseMethod 解析 service 方法行,如:
//
//	repo.ListReposResp RepoList(1: repo.ListReposReq req) (api.get="/api/v1/repos")
func parseMethod(line string) *thriftMethod {
	// 提取 api.get/api.post 注解
	httpMethod := ""
	path := ""
	for _, anno := range []string{"api.get", "api.post"} {
		if val := extractAnnotation(line, anno); val != "" {
			switch anno {
			case "api.get":
				httpMethod = "GET"
			case "api.post":
				httpMethod = "POST"
			}
			path = strings.Trim(val, "\"")
		}
	}
	if httpMethod == "" || path == "" {
		return nil
	}

	// 提取返回类型和方法名: returnType MethodName(
	parenIdx := strings.Index(line, "(")
	if parenIdx < 0 {
		return nil
	}
	sig := strings.TrimSpace(line[:parenIdx])
	sigParts := strings.Fields(sig)
	if len(sigParts) < 2 {
		return nil
	}
	returnType := sigParts[0]
	methodName := sigParts[1]

	// 提取请求类型: (1: type name)
	reqType := ""
	if idx := strings.Index(line, "1: "); idx >= 0 {
		fields := strings.Fields(line[idx+3:])
		if len(fields) >= 1 {
			reqType = fields[0]
		}
	}

	return &thriftMethod{
		Name:       methodName,
		ReturnType: returnType,
		ReqType:    reqType,
		HTTPMethod: httpMethod,
		Path:       path,
	}
}

// extractAnnotation 从注解字符串中提取指定注解的值。
func extractAnnotation(s, key string) string {
	// 查找 key="value" 或 key='value'
	pattern := key + `="`
	idx := strings.Index(s, pattern)
	if idx < 0 {
		pattern = key + `='`
		idx = strings.Index(s, pattern)
		if idx < 0 {
			return ""
		}
	}
	start := idx + len(pattern)
	end := strings.Index(s[start:], s[start-1:start])
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}

// ---------- OpenAPI 生成 ----------

// thriftToOpenAPIType 将 Thrift 类型映射为 OpenAPI 类型/格式。
func thriftToOpenAPIType(thriftType string, structs map[string]*thriftStruct) map[string]any {
	thriftType = strings.TrimSpace(thriftType)

	// list<T>
	if strings.HasPrefix(thriftType, "list<") {
		inner := strings.TrimSuffix(strings.TrimPrefix(thriftType, "list<"), ">")
		return map[string]any{"type": "array", "items": thriftToOpenAPIType(inner, structs)}
	}
	// map<K,V>
	if strings.HasPrefix(thriftType, "map<") {
		inner := strings.TrimSuffix(strings.TrimPrefix(thriftType, "map<"), ">")
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) == 2 {
			return map[string]any{
				"type": "object",
				"additionalProperties": thriftToOpenAPIType(strings.TrimSpace(parts[1]), structs),
			}
		}
		return map[string]any{"type": "object"}
	}

	switch thriftType {
	case "string":
		return map[string]any{"type": "string"}
	case "i32":
		return map[string]any{"type": "integer", "format": "int32"}
	case "i64":
		return map[string]any{"type": "integer", "format": "int64"}
	case "bool":
		return map[string]any{"type": "boolean"}
	case "double":
		return map[string]any{"type": "number", "format": "double"}
	case "binary":
		return map[string]any{"type": "string", "format": "binary"}
	}

	// 自定义 struct:用 $ref 引用
	// 去掉模块前缀(如 repo.RepoInfo → RepoInfo)
	shortName := thriftType
	if idx := strings.LastIndex(thriftType, "."); idx >= 0 {
		shortName = thriftType[idx+1:]
	}
	if _, ok := structs[shortName]; ok {
		return map[string]any{"$ref": "#/components/schemas/" + shortName}
	}

	// 未知类型:当 string 处理
	return map[string]any{"type": "string"}
}

// structToSchema 将 thrift struct 转为 OpenAPI schema。
func structToSchema(ts *thriftStruct, structs map[string]*thriftStruct) map[string]any {
	props := make(map[string]any)
	for _, f := range ts.Fields {
		schema := thriftToOpenAPIType(f.Type, structs)
		// query 参数名可能与 JSON 名不同
		propName := f.JSONName
		if f.Query != "" {
			propName = f.Query
		}
		props[propName] = schema
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
	}
}

func main() {
	idlDir := "idl"
	if len(os.Args) > 1 {
		idlDir = os.Args[1]
	}
	outPath := "docs/openapi.json"
	if len(os.Args) > 2 {
		outPath = os.Args[2]
	}

	// 解析所有 .thrift 文件
	entries, err := os.ReadDir(idlDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 IDL 目录失败: %v\n", err)
		os.Exit(1)
	}

	// 合并所有文件的 struct 定义
	allStructs := make(map[string]*thriftStruct)
	var allMethods []thriftMethod

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".thrift") {
			continue
		}
		tf, err := parseThriftFile(filepath.Join(idlDir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 %s 失败: %v\n", entry.Name(), err)
			continue
		}
		for name, s := range tf.Structs {
			allStructs[name] = s
		}
		for _, methods := range tf.Services {
			allMethods = append(allMethods, methods...)
		}
	}

	// 构建 components/schemas
	schemas := make(map[string]any)
	for name, ts := range allStructs {
		if name == "Empty" {
			continue
		}
		schemas[name] = structToSchema(ts, allStructs)
	}

	// 构建 paths
	paths := make(map[string]any)
	for _, m := range allMethods {
		pathItem, ok := paths[m.Path].(map[string]any)
		if !ok {
			pathItem = make(map[string]any)
			paths[m.Path] = pathItem
		}

		// 构建 request body / parameters
		op := make(map[string]any)
		op["operationId"] = m.Name
		op["tags"] = []string{guessTag(m.Path)}

		reqType := m.ReqType
		if idx := strings.LastIndex(reqType, "."); idx >= 0 {
			reqType = reqType[idx+1:]
		}

		if m.HTTPMethod == "POST" {
			// POST: request body = ReqType struct
			schemaRef := thriftToOpenAPIType(reqType, allStructs)
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{"schema": schemaRef},
				},
			}
		} else {
			// GET: query parameters from ReqType fields
			if ts, ok := allStructs[reqType]; ok {
				var params []map[string]any
				for _, f := range ts.Fields {
					propName := f.Query
					if propName == "" {
						propName = f.JSONName
					}
					schema := thriftToOpenAPIType(f.Type, allStructs)
					params = append(params, map[string]any{
						"name":     propName,
						"in":       "query",
						"schema":   schema,
						"required": false,
					})
				}
				if len(params) > 0 {
					op["parameters"] = params
				}
			}
		}

		// 构建响应
		respType := m.ReturnType
		if idx := strings.LastIndex(respType, "."); idx >= 0 {
			respType = respType[idx+1:]
		}
		respSchema := thriftToOpenAPIType(respType, allStructs)
		op["responses"] = map[string]any{
			"200": map[string]any{
				"description": "Success",
				"content": map[string]any{
					"application/json": map[string]any{"schema": respSchema},
				},
			},
		}

		lowerMethod := strings.ToLower(m.HTTPMethod)
		pathItem[lowerMethod] = op
	}

	// 手动追加自定义路由(非 IDL 定义)
	addCustomPaths(paths)

	// 排序 paths key
	pathKeys := make([]string, 0, len(paths))
	for k := range paths {
		pathKeys = append(pathKeys, k)
	}
	sort.Strings(pathKeys)
	sortedPaths := make(map[string]any)
	for _, k := range pathKeys {
		sortedPaths[k] = paths[k]
	}

	// 组装 OpenAPI 3.0 spec
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "GitFerry API",
			"description": "GitFerry:多平台 Git 同步/镜像/备份 + AI 运维助手。认证方式:请求头 X-API-Key。",
			"version":     "1.15.0",
		},
		"servers": []map[string]any{
			{"url": "http://localhost:8890", "description": "本地开发"},
		},
		"paths": sortedPaths,
		"components": map[string]any{
			"schemas":         schemas,
			"securitySchemes": map[string]any{"ApiKeyAuth": map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"}},
		},
		"security": []map[string]any{{"ApiKeyAuth": []any{}}},
	}

	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化 OpenAPI 失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil { //nolint:gosec // 路径来自命令行参数,由部署方控制
		fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, out, 0o600); err != nil { //nolint:gosec // 同上
		fmt.Fprintf(os.Stderr, "写入 %s 失败: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf(">> 生成 %s(paths=%d, schemas=%d)\n", outPath, len(sortedPaths), len(schemas))
}

// guessTag 从路径推断 API 标签(分组名)。
func guessTag(path string) string {
	switch {
	case strings.Contains(path, "/repos"):
		return "Repo"
	case strings.Contains(path, "/sync/"):
		return "SyncTask"
	case strings.Contains(path, "/webhook/"):
		return "Webhook"
	case strings.Contains(path, "/logs/"):
		return "OperationLog"
	case strings.Contains(path, "/platform"):
		return "Platform"
	case strings.Contains(path, "/system"):
		return "System"
	case strings.Contains(path, "/mirror"):
		return "Mirror"
	case strings.Contains(path, "/ai/"):
		return "AI"
	default:
		return "Other"
	}
}

// addCustomPaths 追加非 IDL 定义的端点(AI/Mirror/Health/Webhook receive)。
func addCustomPaths(paths map[string]any) {
	// Health
	paths["/health"] = map[string]any{
		"get": map[string]any{
			"tags":   []string{"Health"},
			"summary": "健康检查(DB/Redis/Service)",
			"responses": map[string]any{
				"200": map[string]any{"description": "所有依赖正常"},
				"503": map[string]any{"description": "有依赖不可用"},
			},
		},
	}
	paths["/ping"] = map[string]any{
		"get": map[string]any{
			"tags": []string{"Health"}, "summary": "探活",
			"responses": map[string]any{"200": map[string]any{"description": "OK"}},
		},
	}
	// Webhook receive
	paths["/api/webhook/receive/{repoKey}"] = map[string]any{
		"post": map[string]any{
			"tags":   []string{"Webhook"}, "summary": "接收平台 Webhook",
			"parameters": []map[string]any{
				{"name": "repoKey", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			},
			"requestBody": map[string]any{
				"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}},
			},
			"responses": map[string]any{
				"200": map[string]any{"description": "接收成功"},
				"401": map[string]any{"description": "签名验证失败"},
				"429": map[string]any{"description": "速率限制"},
			},
		},
	}
	// AI
	paths["/api/v1/ai/status"] = map[string]any{
		"get": map[string]any{
			"tags": []string{"AI"}, "summary": "AI 助手状态",
			"responses": map[string]any{
				"200": map[string]any{"description": "AI 已启用"},
				"501": map[string]any{"description": "AI 未启用"},
			},
		},
	}
	paths["/api/v1/ai/chat"] = map[string]any{
		"post": map[string]any{
			"tags":       []string{"AI"},
			"summary":    "AI 对话(SSE 流式)",
			"description": "POST SSE 流式响应。事件: start → delta*/tool_start/tool_end/tool_confirm → done|error。危险操作需前端确认卡。",
			"requestBody": map[string]any{
				"content": map[string]any{"application/json": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"session_id": map[string]any{"type": "string", "description": "会话 ID(空=新建)"},
							"message":    map[string]any{"type": "string", "description": "用户消息"},
							"confirmed_tool_call": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"tool":  map[string]any{"type": "string"},
									"token": map[string]any{"type": "string", "description": "tool_confirm 返回的确认令牌"},
								},
							},
						},
					},
				}},
			},
			"responses": map[string]any{
				"200": map[string]any{"description": "SSE 流", "content": map[string]any{"text/event-stream": map[string]any{}}},
				"401": map[string]any{"description": "未认证"},
				"429": map[string]any{"description": "并发上限"},
				"501": map[string]any{"description": "AI 未启用"},
			},
		},
	}
	// Mirror
	for _, p := range []string{
		"/api/v1/mirror/channels", "/api/v1/mirror/runs", "/api/v1/mirror/targets",
	} {
		paths[p] = map[string]any{
			"get": map[string]any{
				"tags": []string{"Mirror"}, "summary": "镜像中心接口(" + p + ")",
				"responses": map[string]any{"200": map[string]any{"description": "Success"}},
			},
			"post": map[string]any{
				"tags": []string{"Mirror"}, "summary": "镜像中心操作(" + p + ")",
				"responses": map[string]any{"200": map[string]any{"description": "Success"}},
			},
		}
	}
}