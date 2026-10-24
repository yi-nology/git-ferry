// 历史同步(histsync)的公开 API 契约。
//
// **为什么放在这里而不是 idl/histsync.thrift**:
//
// histsync 的路由是**手写**在 biz/router/git_sync/git_sync.go 里的
// (hz 生成的文件里手工追加的段落),不是从 thrift IDL 生成的 ——
// 就像镜像中心(mirror)/AI 助手/运维(ops)一样,属于「非 IDL 定义端点」。
// cmd/apidoc 的 addCustomPaths 本来就是这类端点的归档地。
//
// 反过来,若新建 idl/histsync.thrift:
//   - thriftgo 只编译 git_sync.thrift(含它的 include),新文件不会被编译,
//     于是它既不生成代码、也不受 thriftgo 校验 —— 只是一份没人验证的副本;
//   - histsync 的请求/响应结构体定义在 git-ferry-core 的 Go struct 里
//     (corebridge.HistoryChannelInput 等),thrift 里再抄一份 = 第二份真相,
//     core 改字段时 thrift 不会跟着动,契约反而开始骗人。
//
// 所以:路径与方法在这里按「路由表」逐条声明(下表与 biz/router/git_sync/git_sync.go
// 的 _histsync 分组一一对应),请求/响应结构体字段名与 core 的 json tag 对齐。
// 字段来源:
//   - 入参:biz/handler/git_sync/histsync_service.go 里各 handler 绑定的 struct;
//   - 出参:corebridge/core 的 DTO(json tag 见 git-ferry-core/service/history_service.go)。
package main

import (
	"strconv"
	"strings"
)

// hsParam 是一个路径/查询参数。
type hsParam struct {
	Name        string
	In          string // "path" | "query"
	Description string
	Type        string // "integer" | "string"
	Format      string
	Required    bool
}

// schema 返回该参数的 OpenAPI schema。
func (p hsParam) schema() map[string]any {
	s := map[string]any{"type": p.Type}
	if p.Format != "" {
		s["format"] = p.Format
	}
	return s
}

// hsOp 是一个 histsync 端点。
type hsOp struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string

	Params []hsParam

	// Body 是请求体的 component schema 名;空 = 该端点不收请求体。
	Body string
	// BodyOptional 为真时请求体可省略(如 /push 的 force_push)。
	BodyOptional bool

	// Response 是 200 响应的 component schema 名。
	Response string
	// ResponseSchema 非空时直接作为 200 响应 schema(用于非 JSON 的响应)。
	ResponseSchema map[string]any
	// ResponseContentType 默认 application/json。
	ResponseContentType string
	// ResponseDescription 覆盖默认的响应说明。
	ResponseDescription string

	// Errors 是该端点可能返回的错误状态码。
	Errors []int
}

// hsErrDescriptions 是错误状态码的说明。
var hsErrDescriptions = map[int]string{
	400: "请求参数不合法(如路径上的 id 不是正整数)",
	403: "角色不足:写操作需 operator/admin,回滚需 admin",
	404: "对象不存在",
}

// historySyncOps 是 histsync 的端点清单。
//
// 清单来源:biz/router/git_sync/git_sync.go 里 `_histsync` 分组的注册行
// (方法、路径、RBAC 守卫),入参/出参来源见 biz/handler/git_sync/histsync_service.go。
var historySyncOps = []hsOp{
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels",
		OperationID: "HistoryChannelList",
		Summary:     "列出历史同步通道",
		Description: "通道是历史同步的配置单元(两侧仓库 + 身份映射 + 推送目标)。",
		Params: []hsParam{
			{Name: "page", In: "query", Type: "integer", Format: "int32", Description: "页码,从 1 开始,默认 1"},
			{Name: "pageSize", In: "query", Type: "integer", Format: "int32", Description: "每页条数,默认 50,上限 200"},
		},
		Response: "HistoryChannelListData",
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels",
		OperationID: "HistoryChannelCreate",
		Summary:     "创建历史同步通道",
		Description: "通道名唯一。source_repo_key/target_repo_key 与 source_dir/target_dir 二选一;" +
			"module_source/module_target 是身份映射(创建时必填),用于把内网 module path 改写成公开形态。",
		Body:     "HistoryChannelInput",
		Response: "HistoryChannelDTO",
		Errors:   []int{400, 403},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}",
		OperationID: "HistoryChannelGet",
		Summary:     "读取历史同步通道详情",
		Description: "含该通道的推送目标(顺序 = 推送优先级:先留底后发布)。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryChannelDTO",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/update",
		OperationID: "HistoryChannelUpdate",
		Summary:     "更新历史同步通道",
		Description: "**部分更新**语义:未出现的字段保持原值,显式给空串表示清空。" +
			"(指针字段区分「未指定」与「显式清空」,避免把身份映射一起抹掉。)",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "HistoryChannelInput",
		Response: "HistoryChannelDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/delete",
		OperationID: "HistoryChannelDelete",
		Summary:     "删除历史同步通道",
		Description: "连同该通道的推送目标一并删除。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryDeletedData",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}/targets",
		OperationID: "HistoryTargetList",
		Summary:     "列出通道的推送目标",
		Description: "顺序即推送优先级(先留底后发布)。凭据不回显,只有 has_credential。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryTargetListData",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/targets",
		OperationID: "HistoryTargetCreate",
		Summary:     "创建推送目标",
		Description: "凭据明文只在传输中出现,落库即加密,响应不回显。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "HistoryTargetInput",
		Response: "HistoryTargetDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/targets/{id}/update",
		OperationID: "HistoryTargetUpdate",
		Summary:     "更新推送目标",
		Description: "可改目标名/远端地址/凭据。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "推送目标 ID"},
		},
		Body:     "HistoryTargetInput",
		Response: "HistoryTargetDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/targets/{id}/delete",
		OperationID: "HistoryTargetDelete",
		Summary:     "删除推送目标",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "推送目标 ID"},
		},
		Response: "HistoryDeletedData",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/plan",
		OperationID: "HistoryChannelPlan",
		Summary:     "生成同步计划(只读)",
		Description: "纯读:按通道算一份同步计划(逐文件处置 + 提交分组 + 统计),不改任何状态、不推送。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryPlanDTO",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/resolve",
		OperationID: "HistoryChannelResolve",
		Summary:     "生成计划并解算 hold 冲突(只读)",
		Description: "在 plan 的基础上计算 hold 文件的自动解法;解不了的会给出冲突卷宗(仍需人工裁决)。纯读。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryPlanDTO",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/apply",
		OperationID: "HistoryChannelApply",
		Summary:     "按通道把计划落到目标树(本地,不推送)",
		Description: "产出执行记录(apply 类型):目标侧既有提交原样保留,新增提交按身份映射改写作者。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryRunDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/preview",
		OperationID: "HistoryChannelPreview",
		Summary:     "执行预检(不推送)",
		Description: "把这次同步的全部影响面摊开:门禁结果、各推送目标的影响面、凭据状态、是否需要强推。纯读,不推送。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryPreviewReport",
		Errors:   []int{400, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}/preview.md",
		OperationID: "HistoryChannelPreviewMarkdown",
		Summary:     "预检报告的 Markdown 视图(给人读)",
		Description: "与 /preview 同源,输出 text/markdown 便于直接贴进评审。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		ResponseSchema:      map[string]any{"type": "string"},
		ResponseContentType: "text/markdown",
		ResponseDescription: "预检报告的 Markdown 文本(直接返回,不是统一信封)。",
		Errors:              []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/push",
		OperationID: "HistoryChannelPush",
		Summary:     "推送最近一次执行记录的产物",
		Description: "把 apply 的产物推到各推送目标。请求体可省略;force_push=true 表示覆盖远端已有提交" +
			"(强推会丢掉远端别人的提交,需 admin 角色)。旧字段名 force 已改名为 force_push,传旧名会明确报错。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:         "HistoryPushInput",
		BodyOptional: true,
		Response:     "HistoryPushDTO",
		Errors:       []int{400, 403, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}/runs",
		OperationID: "HistoryRunList",
		Summary:     "列出通道的执行记录",
		Description: "记录类别:apply(内容移植)/ push(推送)/ rollback(回滚)。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
			{Name: "page", In: "query", Type: "integer", Format: "int32", Description: "页码,从 1 开始,默认 1"},
			{Name: "pageSize", In: "query", Type: "integer", Format: "int32", Description: "每页条数,默认 50,上限 200"},
		},
		Response: "HistoryRunListData",
		Errors:   []int{400, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}/audit",
		OperationID: "HistoryChannelAudit",
		Summary:     "校验通道内执行记录的哈希链",
		Description: "与运维侧审计链同构,但链在通道内:通道被删则链随之消失,不会无限增长。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryChainResult",
		Errors:   []int{400, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/channels/{id}/overrides",
		OperationID: "HistoryOverrideList",
		Summary:     "列出通道的裁决",
		Description: "裁决是对 hold 文件的人工/AI 拍板,决定它取源侧、取目标侧还是合并。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Response: "HistoryOverrideListData",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/overrides",
		OperationID: "HistoryOverrideSet",
		Summary:     "写入(或覆盖)一条裁决",
		Description: "verdict 取 keep_target / take_source / merged;merged 时 content 必填。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "HistoryOverrideInput",
		Response: "HistoryOverrideDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/overrides/{id}/delete",
		OperationID: "HistoryOverrideDelete",
		Summary:     "删除一条裁决",
		Description: "删除后该文件回到自动判定。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "裁决 ID"},
		},
		Response: "HistoryDeletedData",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/targets/{id}/rotate",
		OperationID: "HistoryTargetRotate",
		Summary:     "轮换推送目标的凭据",
		Description: "**只换密钥**:目标名与远端地址不动 —— 换密钥是常规运维动作,不该有机会顺手改掉推送到哪里。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "推送目标 ID"},
		},
		Body:     "HistoryCredentialInput",
		Response: "HistoryTargetDTO",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/channels/{id}/sync",
		OperationID: "HistorySync",
		Summary:     "一次跑完「预检 → 移植 → 推送」",
		Description: "有阻塞项(门禁不过 / 有待裁决冲突 / 需要强推)时**停下并报告**,不往下走。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "通道 ID"},
		},
		Body:     "HistorySyncInput",
		Response: "HistorySyncReport",
		Errors:   []int{400, 403, 404},
	},
	{
		Method:      "GET",
		Path:        "/api/v1/histsync/runs/{id}",
		OperationID: "HistoryRunGet",
		Summary:     "读取执行记录详情",
		Description: "审计材料:计划统计 / commit 映射 / 痕迹检测报告 / ref 对照。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "执行记录 ID"},
		},
		Response: "HistoryRunDTO",
		Errors:   []int{400, 404},
	},
	{
		Method:      "POST",
		Path:        "/api/v1/histsync/runs/{id}/rollback",
		OperationID: "HistoryRunRollback",
		Summary:     "把目标树回滚到指定 ref",
		Description: "危险操作:会把目标仓推到回滚后的状态。ref 通常取同步前的 HEAD。需要 admin 角色。",
		Params: []hsParam{
			{Name: "id", In: "path", Type: "integer", Format: "int64", Required: true, Description: "执行记录 ID"},
		},
		Body:     "HistoryRollbackInput",
		Response: "HistoryRollbackDTO",
		Errors:   []int{400, 403, 404},
	},
}

// addHistorySyncPaths 把 histsync 端点写进 OpenAPI 的 paths 与 components。
func addHistorySyncPaths(paths map[string]any, schemas map[string]any) {
	for name, schema := range historySyncSchemas() {
		schemas[name] = schema
	}
	for _, op := range historySyncOps {
		pathItem, ok := paths[op.Path].(map[string]any)
		if !ok {
			pathItem = make(map[string]any)
			paths[op.Path] = pathItem
		}
		// OpenAPI 的 path item 键用小写方法名(与 IDL 生成的路径保持一致)。
		pathItem[strings.ToLower(op.Method)] = op.build()
	}
}

// build 把一个端点渲染成 OpenAPI operation。
func (op hsOp) build() map[string]any {
	operation := map[string]any{
		"operationId": op.OperationID,
		"tags":        []string{"HistorySync"},
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

	contentType := op.ResponseContentType
	if contentType == "" {
		contentType = "application/json"
	}
	responseSchema := op.ResponseSchema
	if responseSchema == nil {
		responseSchema = map[string]any{"$ref": "#/components/schemas/" + op.Response}
	}
	responseDescription := op.ResponseDescription
	if responseDescription == "" {
		responseDescription = "成功。响应体是统一信封 {code, message, data, timestamp},此处 schema 描述 data 部分。"
	}
	responses := map[string]any{
		"200": map[string]any{
			"description": responseDescription,
			"content": map[string]any{
				contentType: map[string]any{"schema": responseSchema},
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

// ---------- schema helpers ----------
//
// sch* 构造小工具在 schema.go(本包共用),不在这里重复定义。

// historySyncSchemas 返回 histsync 用到的 component schema。
//
// 字段名与 core 的 json tag 严格一致(注意 history.Summary 一族在 core 里
// 没有 json tag,线上键名就是 Go 字段名 —— 这里是照抄实际线上形状)。
func historySyncSchemas() map[string]any {
	return map[string]any{
		// ---------- 请求体 ----------
		"HistoryChannelInput": schObj(map[string]any{
			"name":            schStr(),
			"source_repo_key": schStr(),
			"target_repo_key": schStr(),
			"source_dir":      schStr(),
			"target_dir":      schStr(),
			"module_source":   schStr(),
			"module_target":   schStr(),
			"host_source":     schStr(),
			"host_target":     schStr(),
			"author_name":     schStr(),
			"author_email":    schStr(),
			"path_rules":      schArr(schRef("HistoryPathRule")),
			"scrub_rules":     schArr(schRef("HistoryScrubRule")),
			"conflict_rules":  schArr(schRef("HistoryConflictRule")),
			"source_branch":   schStr(),
			"target_branch":   schStr(),
			"enabled":         schBool(),
		}),
		"HistoryPathRule": schObj(map[string]any{
			"from": schStr(),
			"to":   schStr(),
		}),
		"HistoryScrubRule": schObj(map[string]any{
			"name":        schStr(),
			"category":    schStr(),
			"pattern":     schStr(),
			"severity":    schStr(),
			"replacement": schStr(),
		}),
		"HistoryConflictRule": schObj(map[string]any{
			"kind":     schStr(),
			"enabled":  schBool(),
			"patterns": schArr(schStr()),
		}),
		"HistoryTargetInput": schObj(map[string]any{
			"name":       schStr(),
			"repo_url":   schStr(),
			"cred_type":  schStr(),
			"credential": schStr(),
			"username":   schStr(),
		}),
		"HistoryOverrideInput": schObj(map[string]any{
			"path":        schStr(),
			"source_path": schStr(),
			"verdict":     schStr(),
			"content":     schStr(),
			"reason":      schStr(),
			"decided_by":  schStr(),
		}),
		"HistoryCredentialInput": schObj(map[string]any{
			"cred_type":  schStr(),
			"credential": schStr(),
			"username":   schStr(),
			"expires_at": schStr(),
		}),
		"HistoryPushInput": schObj(map[string]any{
			"force_push": schBool(),
		}),
		"HistorySyncInput": schObj(map[string]any{
			"force":      schBool(),
			"force_push": schBool(),
		}),
		"HistoryRollbackInput": schObj(map[string]any{
			"ref": schStr(),
		}),

		// ---------- 出参 ----------
		"HistoryChannelDTO": schObj(map[string]any{
			"id":         schInt64(),
			"name":       schStr(),
			"enabled":    schBool(),
			"created_at": schStr(),
			"updated_at": schStr(),
			"targets":    schArr(schRef("HistoryTargetDTO")),
		}),
		"HistoryTargetDTO": schObj(map[string]any{
			"id":             schInt64(),
			"channel_id":     schInt64(),
			"name":           schStr(),
			"repo_url":       schStr(),
			"cred_type":      schStr(),
			"username":       schStr(),
			"has_credential": schBool(),
		}),
		"HistoryOverrideDTO": schObj(map[string]any{
			"id":          schInt64(),
			"channel_id":  schInt64(),
			"path":        schStr(),
			"source_path": schStr(),
			"verdict":     schStr(),
			"content":     schStr(),
			"reason":      schStr(),
			"decided_by":  schStr(),
			"created_at":  schStr(),
		}),
		"HistoryRunDTO": schObj(map[string]any{
			"id":            schInt64(),
			"channel_id":    schInt64(),
			"kind":          schStr(),
			"pushed_run_id": schInt64(),
			"status":        schStr(),
			"base_head":     schStr(),
			"head":          schStr(),
			"plan_summary":  schStr(),
			"commits":       schStr(),
			"files":         schStr(),
			"skipped":       schStr(),
			"scrub_report":  schStr(),
			"gate_report":   schStr(),
			"refs_before":   schStr(),
			"refs_after":    schStr(),
			"error":         schStr(),
			"prev_hash":     schStr(),
			"entry_hash":    schStr(),
			"started_at":    schStr(),
			"finished_at":   schStr(),
			"duration_ms":   schInt64(),
			"created_at":    schStr(),
		}),
		"HistoryPushDTO": schObj(map[string]any{
			"run_id": schInt64(),
			"pushed": schArr(schStr()),
			"heads":  schStrMap(),
			"error":  schStr(),
		}),
		"HistoryRollbackDTO": schObj(map[string]any{
			"run_id": schInt64(),
			"ref":    schStr(),
			"pushed": schArr(schStr()),
			"heads":  schStrMap(),
		}),
		"HistoryChainResult": schObj(map[string]any{
			"ok":        schBool(),
			"checked":   schInt64(),
			"broken_at": schInt64(),
			"message":   schStr(),
		}),
		"HistoryPlanDTO": schObj(map[string]any{
			"channel_id":  schInt64(),
			"summary":     schRef("HistorySummary"),
			"files":       schArr(schRef("HistoryFilePlan")),
			"commits":     schArr(schRef("HistoryCommitPlan")),
			"resolutions": map[string]any{"type": "object", "additionalProperties": schRef("HistoryResolution")},
			"dossiers":    schArr(schRef("HistoryConflictDossier")),
		}),
		"HistoryPreviewReport": schObj(map[string]any{
			"channel_id":        schInt64(),
			"channel_name":      schStr(),
			"summary":           schRef("HistorySummary"),
			"base_head":         schStr(),
			"new_head":          schStr(),
			"commits":           schArr(schRef("HistoryAppliedCommit")),
			"applied_files":     schArr(schStr()),
			"skipped_files":     schArr(schStr()),
			"protected_count":   schInt64(),
			"pending_conflicts": schArr(schRef("HistoryConflictDossier")),
			"scrub_hits":        schArr(schRef("HistoryScrubHit")),
			"gate":              schRef("HistoryGateReport"),
			"targets":           schArr(schRef("HistoryTargetImpact")),
			"credentials":       schArr(schRef("HistoryCredentialStatus")),
			"needs_force":       schBool(),
			"warnings":          schArr(schStr()),
			"plan_markdown":     schStr(),
		}),
		"HistorySyncReport": schObj(map[string]any{
			"preview": schRef("HistoryPreviewReport"),
			"apply":   schRef("HistoryRunDTO"),
			"push":    schRef("HistoryPushDTO"),
			"blocked": schArr(schStr()),
		}),
		"HistoryGateReport": schObj(map[string]any{
			"ok":               schBool(),
			"gofmt_violations": schArr(schStr()),
			"type_check_err":   schStr(),
			"note":             schStr(),
		}),
		"HistoryTargetImpact": schObj(map[string]any{
			"name":         schStr(),
			"repo_url":     schStr(),
			"remote_head":  schStr(),
			"base_head":    schStr(),
			"new_head":     schStr(),
			"fast_forward": schBool(),
			"diverged":     schBool(),
			"unknown":      schBool(),
			"note":         schStr(),
		}),
		"HistoryCredentialStatus": schObj(map[string]any{
			"target_id":      schInt64(),
			"name":           schStr(),
			"has_credential": schBool(),
			"updated_at":     schStr(),
			"expires_at":     schStr(),
			"expired":        schBool(),
			"expiring_soon":  schBool(),
			"note":           schStr(),
		}),

		// ---------- 计划内部结构 ----------
		// history.Summary 在 core 里没有 json tag,线上键名即 Go 字段名。
		"HistorySummary": schObj(map[string]any{
			"Total":         schInt64(),
			"Same":          schInt64(),
			"Port":          schInt64(),
			"PortWithScrub": schInt64(),
			"Exclude":       schInt64(),
			"Hold":          schInt64(),
			"Resolved":      schInt64(),
			"Preserve":      schInt64(),
			"ScrubHits":     schInt64(),
			"AddedLines":    schInt64(),
			"RemovedLines":  schInt64(),
		}),
		"HistoryFilePlan": schObj(map[string]any{
			"Path":          schStr(),
			"SourcePath":    schStr(),
			"Disposition":   schStr(),
			"Reason":        schStr(),
			"ScrubHits":     schArr(schRef("HistoryScrubHit")),
			"OriginCommit":  schStr(),
			"OriginSubject": schStr(),
			"AddedLines":    schInt64(),
			"RemovedLines":  schInt64(),
		}),
		"HistoryCommitPlan": schObj(map[string]any{
			"Hash":        schStr(),
			"Subject":     schStr(),
			"Files":       schArr(schStr()),
			"Disposition": schStr(),
		}),
		"HistoryResolution": schObj(map[string]any{
			"Path":      schStr(),
			"Verdict":   schStr(),
			"Content":   schStr(),
			"BaseFound": schBool(),
			"Reason":    schStr(),
			"Base":      schStr(),
			"Ours":      schStr(),
			"Theirs":    schStr(),
			"Merged":    schStr(),
			"Conflicts": schInt64(),
		}),
		"HistoryConflictDossier": schObj(map[string]any{
			"Path":       schStr(),
			"SourcePath": schStr(),
			"Verdict":    schStr(),
			"Reason":     schStr(),
			"Base":       schStr(),
			"Ours":       schStr(),
			"Theirs":     schStr(),
			"Merged":     schStr(),
			"Conflicts":  schInt64(),
			"OursOnly":   schArr(schStr()),
			"TheirsOnly": schArr(schStr()),
		}),
		"HistoryScrubHit": schObj(map[string]any{
			"Rule":      schStr(),
			"Category":  schStr(),
			"Line":      schInt64(),
			"Text":      schStr(),
			"Mandatory": schBool(),
		}),
		"HistoryAppliedCommit": schObj(map[string]any{
			"SourceCommit":  schStr(),
			"SourceSubject": schStr(),
			"TargetCommit":  schStr(),
			"Files":         schArr(schStr()),
		}),

		// ---------- 列表包装 ----------
		"HistoryChannelListData": schObj(map[string]any{
			"list":  schArr(schRef("HistoryChannelDTO")),
			"total": schInt64(),
		}),
		"HistoryTargetListData": schObj(map[string]any{
			"list": schArr(schRef("HistoryTargetDTO")),
		}),
		"HistoryOverrideListData": schObj(map[string]any{
			"list": schArr(schRef("HistoryOverrideDTO")),
		}),
		"HistoryRunListData": schObj(map[string]any{
			"list":  schArr(schRef("HistoryRunDTO")),
			"total": schInt64(),
		}),
		"HistoryDeletedData": schObj(map[string]any{
			"deleted": schInt64(),
		}),
	}
}
