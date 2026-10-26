// 镜像中心(mirror)的公开 API 契约。
//
// **为什么放在这里而不是 idl/mirror.thrift**:idl/mirror.thrift 是**生成物**
// （`hz model` 产出 `biz/model/mirror`），它描述的是**请求/响应的数据结构**；
// 而这些端点本身是**手写路由**（`biz/router/git_sync/git_sync.go` 的 `_mirror` 分组），
// 与 histsync / AI / ops 同属「非 IDL 定义端点」——
// `cmd/apidoc` 的 `addCustomPaths` 本来就是这类端点的归档地。
//
// 字段来源:
//   - 入参:`biz/handler/git_sync/mirror_service.go` 各 handler 绑定的 struct;
//   - 出参:corebridge/core 的 DTO(json tag 见 git-ferry-core/model/mirror_channel.go)。
//
// 本文件与 `histsync.go` 同款：**路径与方法按路由表逐条声明**，
// 不引入第二份真相（thrift 里再抄一遍 core 的结构体 = core 改字段时
// 契约不会跟着动，契约反而开始骗人）。
package main

import (
	"strconv"
	"strings"
)

// mirrorParam 是一个路径/查询参数。
type mirrorParam struct {
	Name        string
	In          string // "path" | "query"
	Description string
	Type        string // "integer" | "string"
	Format      string
	Required    bool
}

// schema 返回该参数的 OpenAPI schema。
func (p *mirrorParam) schema() map[string]any {
	s := map[string]any{"type": p.Type}
	if p.Format != "" {
		s["format"] = p.Format
	}
	return s
}

// mirrorOp 是一个镜像中心端点。
type mirrorOp struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string

	Params []mirrorParam

	// Body 是请求体的 component schema 名;空 = 该端点不收请求体。
	Body string
	// BodyOptional 为真时请求体可省略(如 /push 的 force_push)。
	BodyOptional bool

	// Response 是响应 data 部分的 component schema 名。
	Response string
	// ResponseDescription 覆盖 200 的描述。
	ResponseDescription string
	// Errors 是可能返回的错误码。
	Errors []int
}

// mirrorOps 逐条声明镜像中心的端点。
//
// 路径与方法与 `biz/router/git_sync/git_sync.go` 的 `_mirror` 分组一一对应
// （由 internal/pkg/swagger 的契约测试守住）。
var mirrorOps = []mirrorOp{
	{
		Method: "GET", Path: "/api/v1/mirror/channels",
		OperationID: "mirrorChannelList",
		Summary:     "列出镜像通道",
		Response:    "MirrorChannelListResp",
	},
	{
		Method: "POST", Path: "/api/v1/mirror/channels",
		OperationID: "mirrorChannelCreate",
		Summary:     "新建镜像通道",
		Description: "publish 模式会先拉取源仓库并读取 go.mod,提前暴露仓库不可达 / module 缺失等问题。",
		Body:        "MirrorCreateChannelReq",
		Response:    "MirrorChannel",
		Errors:      []int{400, 500},
	},
	{
		Method: "GET", Path: "/api/v1/mirror/channels/{id}",
		OperationID: "mirrorChannelGet",
		Summary:     "镜像通道详情(含推送目标)",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "MirrorChannel",
		Errors:   []int{404},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/channels/{id}/delete",
		OperationID: "mirrorChannelDelete",
		Summary:     "删除镜像通道",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "MirrorOK",
		Errors:   []int{404, 500},
	},
	{
		Method: "GET", Path: "/api/v1/mirror/channels/{id}/versions",
		OperationID: "mirrorVersions",
		Summary:     "列出源仓的 tag 与各目标已发布版本",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "MirrorVersionsResult",
		Errors:   []int{404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/channels/{id}/preview",
		OperationID: "mirrorPreview",
		Summary:     "预检发布(不推送)",
		Description: "校验身份改写与编译门禁,给出将产生的提交;不推送。",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "MirrorPreviewReq",
		Response: "MirrorTagReport",
		Errors:   []int{400, 404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/channels/{id}/runs",
		OperationID: "mirrorRunCreate",
		Summary:     "创建发布任务(异步执行)",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "MirrorRunReq",
		Response: "MirrorRun",
		Errors:   []int{400, 404, 500},
	},
	{
		Method: "GET", Path: "/api/v1/mirror/channels/{id}/runs",
		OperationID: "mirrorRunList",
		Summary:     "列出通道的发布任务",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "MirrorRunListResp",
		Errors:   []int{404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/targets/{id}/update",
		OperationID: "mirrorTargetUpdate",
		Summary:     "更新推送目标",
		Description: "凭据传空串表示保持原凭据(不必重传)。SSH 主机密钥钉扎字段可在此配置。",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "目标 ID"},
		},
		Body:     "MirrorUpdateTargetReq",
		Response: "MirrorTarget",
		Errors:   []int{400, 404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/targets/{id}/delete",
		OperationID: "mirrorTargetDelete",
		Summary:     "删除推送目标",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "目标 ID"},
		},
		Response: "MirrorOK",
		Errors:   []int{404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/targets/{id}/test",
		OperationID: "mirrorTargetTest",
		Summary:     "测试目标连通性",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "目标 ID"},
		},
		Response: "MirrorConnTest",
		Errors:   []int{404, 500},
	},
	{
		Method: "POST", Path: "/api/v1/mirror/channels/{id}/verify",
		OperationID: "mirrorVerify",
		Summary:     "校验发布身份一致性",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "MirrorVerifyReq",
		Response: "MirrorVerifyResult",
		Errors:   []int{400, 404, 500},
	},
	{
		Method: "GET", Path: "/api/v1/mirror/runs/{id}",
		OperationID: "mirrorRunGet",
		Summary:     "发布任务详情",
		Params: []mirrorParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "发布任务 ID"},
		},
		Response: "MirrorRun",
		Errors:   []int{404},
	},
}

// addMirrorPaths 把镜像中心的端点写进 OpenAPI。
func addMirrorPaths(paths, schemas map[string]any) {
	for name, schema := range mirrorSchemas() {
		schemas[name] = schema
	}
	for i := range mirrorOps {
		op := &mirrorOps[i]
		pathItem, ok := paths[op.Path].(map[string]any)
		if !ok {
			pathItem = make(map[string]any)
			paths[op.Path] = pathItem
		}
		pathItem[strings.ToLower(op.Method)] = op.build()
	}
}

// build 把一个端点渲染成 OpenAPI operation。
func (op *mirrorOp) build() map[string]any {
	operation := map[string]any{
		"operationId": op.OperationID,
		"tags":        []string{"Mirror"},
		"summary":     op.Summary,
	}
	if op.Description != "" {
		operation["description"] = op.Description
	}
	if len(op.Params) > 0 {
		params := make([]map[string]any, 0, len(op.Params))
		for _, p := range op.Params {
			param := map[string]any{
				"name":     p.Name,
				"in":       p.In,
				"required": p.Required,
				"schema":   p.schema(),
			}
			if p.Description != "" {
				param["description"] = p.Description
			}
			params = append(params, param)
		}
		operation["parameters"] = params
	}
	if op.Body != "" {
		operation["requestBody"] = map[string]any{
			"required": !op.BodyOptional,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/" + op.Body},
				},
			},
		}
	}
	description := op.ResponseDescription
	if description == "" {
		description = "成功。响应体是统一信封 {code, message, data, timestamp},此处 schema 描述 data 部分。"
	}
	responses := map[string]any{
		"200": map[string]any{
			"description": description,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/" + op.Response},
				},
			},
		},
	}
	for _, code := range op.Errors {
		responses[strconv.Itoa(code)] = map[string]any{
			"description": hsErrDescriptions[code],
		}
	}
	operation["responses"] = responses
	return operation
}

// mirrorSchemas 返回镜像中心用到的 component schema。
//
// 字段名与 core 的 json tag 一致（core 在 model/mirror_channel.go）。
func mirrorSchemas() map[string]any {
	return map[string]any{
		"MirrorOK": schObj(map[string]any{}),
		"MirrorChannel": schObj(map[string]any{
			"id":      schInt64(),
			"name":    schStr(),
			"mode":    schStr(),
			"repoKey": schStr(),
			"module":  schStr(),
			"scrubRules": schArr(schObj(map[string]any{
				"name":        schStr(),
				"category":    schStr(),
				"pattern":     schStr(),
				"severity":    schStr(),
				"replacement": schStr(),
			})),
			"targets": schArr(schObj(map[string]any{
				"id":                 schInt64(),
				"channelId":          schInt64(),
				"remote":             schStr(),
				"repoUrl":            schStr(),
				"targetModule":       schStr(),
				"credType":           schStr(),
				"username":           schStr(),
				"hasCredential":      schBool(),
				"insecureSkipTLS":    schBool(),
				"hostKeyFingerprint": schStr(),
				"knownHostsPath":     schStr(),
			})),
		}),
		"MirrorChannelListResp": schObj(map[string]any{
			"total":    schInt64(),
			"channels": schArr(schRef("MirrorChannel")),
		}),
		"MirrorRunListResp": schObj(map[string]any{
			"total": schInt64(),
			"runs":  schArr(schRef("MirrorRun")),
		}),
		"MirrorTarget": schObj(map[string]any{
			"id":                 schInt64(),
			"channelId":          schInt64(),
			"remote":             schStr(),
			"repoUrl":            schStr(),
			"targetModule":       schStr(),
			"credType":           schStr(),
			"username":           schStr(),
			"hasCredential":      schBool(),
			"insecureSkipTLS":    schBool(),
			"hostKeyFingerprint": schStr(),
			"knownHostsPath":     schStr(),
		}),
		"MirrorRun": schObj(map[string]any{
			"id":           schInt64(),
			"channelId":    schInt64(),
			"targetId":     schInt64(),
			"mode":         schStr(),
			"status":       schStr(),
			"sourceCommit": schStr(),
			"targetCommit": schStr(),
			"error":        schStr(),
			"createdAt":    schStr(),
		}),
		"MirrorCreateChannelReq": schObj(map[string]any{
			"name":    schStr(),
			"mode":    schStr(),
			"repoKey": schStr(),
			"targets": schArr(schRef("MirrorTargetReq")),
			// 痕迹清理规则:发布前把内容里的内网痕迹清掉
			// (身份改写只管 module path)。
			"scrubRules": schArr(schObj(map[string]any{
				"name":        schStr(),
				"category":    schStr(),
				"pattern":     schStr(),
				"severity":    schStr(),
				"replacement": schStr(),
			})),
		}),
		"MirrorTargetReq": schObj(map[string]any{
			"remote":       schStr(),
			"repoUrl":      schStr(),
			"targetModule": schStr(),
			"credType":     schStr(),
			"credential":   schStr(),
			"username":     schStr(),
			// SSH 主机密钥钉扎
			"insecureSkipTLS":    schBool(),
			"hostKeyFingerprint": schStr(),
			"knownHostsPath":     schStr(),
		}),
		"MirrorUpdateTargetReq": schObj(map[string]any{
			"remote":             schStr(),
			"repoUrl":            schStr(),
			"targetModule":       schStr(),
			"credType":           schStr(),
			"credential":         schStr(),
			"username":           schStr(),
			"insecureSkipTLS":    schBool(),
			"hostKeyFingerprint": schStr(),
			"knownHostsPath":     schStr(),
		}),
		"MirrorPreviewReq": schObj(map[string]any{
			"targetId": schInt64(),
			"tag":      schStr(),
		}),
		"MirrorRunReq": schObj(map[string]any{
			"targetId":       schInt64(),
			"tags":           schArr(schStr()),
			"allowOverwrite": schBool(),
		}),
		"MirrorVerifyReq": schObj(map[string]any{
			"targetId": schInt64(),
		}),
		"MirrorVerifyResult": schObj(map[string]any{
			"ok":      schBool(),
			"message": schStr(),
		}),
		"MirrorConnTest": schObj(map[string]any{
			"connected": schBool(),
			"message":   schStr(),
		}),
		"MirrorVersionsResult": schObj(map[string]any{
			"sourceTags": schArr(schStr()),
			"targets":    schArr(schObj(map[string]any{})),
		}),
		"MirrorTagReport": schObj(map[string]any{
			"tag":                 schStr(),
			"sourceCommit":        schStr(),
			"tree":                schStr(),
			"commit":              schStr(),
			"replacedFiles":       schArr(schStr()),
			"residualFiles":       schArr(schStr()),
			"scrubHits":           schArr(schObj(map[string]any{})),
			"verifyCmd":           schStr(),
			"remoteTagExisted":    schBool(),
			"treeMatchedRemote":   schBool(),
			"overwroteDivergent":  schBool(),
			"tagAlreadyPublished": schBool(),
			"h1WillChange":        schBool(),
			"warning":             schStr(),
		}),
	}
}
