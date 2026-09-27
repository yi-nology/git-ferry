package tools

import (
	"context"
	"fmt"
	"strings"
)

// MemStore 记忆库接口(tools 不直接依赖 agent/memory 包,保持依赖方向)。
type MemStore interface {
	Remember(kind, content string, tags []string) (id string, err error)
	Recall(query string, limit int) []MemEntry
	Forget(id string) bool
}

// MemEntry 记忆条目视图。
type MemEntry struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
}

// SetMemoryStore 注入记忆库(可选,未注入时工具返回说明)。
func (r *Registry) SetMemoryStore(m MemStore) { r.mem = m }

type rememberInput struct {
	Kind    string   `json:"kind" jsonschema:"类型:preference|pattern|fact|task"`
	Content string   `json:"content" jsonschema:"要记住的内容,一句话说清"`
	Tags    []string `json:"tags,omitempty" jsonschema:"标签,便于检索"`
}

func (r *Registry) rememberIn(ctx context.Context, in rememberInput) (string, error) {
	if r.mem == nil {
		return errJSON("记忆库未启用", nil), nil
	}
	kind := in.Kind
	switch kind {
	case "preference", "pattern", "fact", "task":
	default:
		kind = "fact"
	}
	id, err := r.mem.Remember(kind, in.Content, in.Tags)
	if err != nil {
		return errJSON("写入记忆失败", err), nil
	}
	return marshalJSON(map[string]any{"status": "ok", "id": id, "kind": kind}), nil
}

type recallInput struct {
	Query string `json:"query,omitempty" jsonschema:"检索关键字,空则返回最近记忆"`
	Limit int    `json:"limit,omitempty" jsonschema:"返回条数,默认 8"`
}

func (r *Registry) recallIn(_ context.Context, in recallInput) (string, error) {
	if r.mem == nil {
		return errJSON("记忆库未启用", nil), nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	return marshalJSON(map[string]any{"items": r.mem.Recall(in.Query, limit)}), nil
}

type forgetInput struct {
	ID string `json:"id" jsonschema:"要删除的记忆 ID"`
}

func (r *Registry) forgetIn(_ context.Context, in forgetInput) (string, error) {
	if r.mem == nil {
		return errJSON("记忆库未启用", nil), nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return errJSON("id 不能为空", nil), nil
	}
	ok := r.mem.Forget(in.ID)
	return marshalJSON(map[string]any{"deleted": ok}), nil
}

// diagnoseInput 诊断一次失败执行。
type diagnoseInput struct {
	RunID uint `json:"run_id" jsonschema:"要诊断的执行记录 ID"`
}

func (r *Registry) diagnoseRunIn(ctx context.Context, in diagnoseInput) (string, error) {
	if in.RunID == 0 {
		return errJSON("run_id 必填", nil), nil
	}
	run, taskKey, err := findRunView(r.svc, ctx, in.RunID)
	if err != nil {
		return msgJSON(map[string]any{"found": false, "message": fmt.Sprintf("未找到 run %d", in.RunID)}), nil
	}
	d := diagnoseRunView(run, taskKey)
	return marshalJSON(d), nil
}

// findRunView 在最近历史中定位 run。
func findRunView(svc SyncService, ctx context.Context, runID uint) (*runView, string, error) {
	const pageSize = 50
	offset := 0
	for page := 0; page < 10; page++ {
		tasks, total, err := svc.ListTasks(ctx, "", offset, pageSize)
		if err != nil {
			return nil, "", err
		}
		for _, t := range tasks {
			runs, _, err := svc.ListHistory(ctx, t.Key, 0, 50)
			if err != nil {
				continue
			}
			for _, rs := range runs {
				if rs.ID != runID {
					continue
				}
				return &runView{
					ID: rs.ID, TaskKey: t.Key, Status: rs.Status,
					ErrorType: rs.ErrorType, ErrorMessage: rs.ErrorMessage,
					Details: rs.Details,
				}, t.Key, nil
			}
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return nil, "", fmt.Errorf("run %d not found", runID)
}

// diagnoseRunView 规则化诊断(与壳层 /ops/diagnose 同源逻辑)。
func diagnoseRunView(run *runView, taskKey string) map[string]any {
	et := run.ErrorType
	msg := strings.ToLower(run.ErrorMessage + " " + run.Details)
	var causes, actions []string
	severity := "warning"
	switch et {
	case "auth":
		severity = "critical"
		causes = append(causes, "访问令牌无效或权限不足")
		actions = append(actions, "检查 access_token 是否过期", "确认 repo/push 权限")
	case "network":
		causes = append(causes, "网络不可达或超时")
		actions = append(actions, "检查平台连通性", "配置 proxy 或调大 default_timeout")
	case "divergent", "conflict":
		severity = "critical"
		causes = append(causes, "目标分支与源分叉,拒绝覆盖")
		actions = append(actions, "对比目标独有提交;确认可丢弃后关 keep_divergent")
	case "config":
		causes = append(causes, "仓库/分支/平台配置不正确")
		actions = append(actions, "核对 repo key 与分支名")
	default:
		if strings.Contains(msg, "lfs") {
			causes = append(causes, "LFS 同步失败")
			actions = append(actions, "安装 git-lfs 或关闭 git_lfs")
		}
		if strings.Contains(msg, "bundle") {
			causes = append(causes, "冷备 bundle 失败")
			actions = append(actions, "检查 backup_dir 可写")
		}
		if strings.Contains(msg, "push") {
			causes = append(causes, "推送被拒绝")
			actions = append(actions, "检查目标分支保护;必要时 git_force")
		}
		if len(causes) == 0 {
			causes = append(causes, "未归类错误")
			actions = append(actions, "查看 get_run_detail 步骤链", "必要时 rebuild_task 全量重建")
		}
	}
	return map[string]any{
		"run_id":        run.ID,
		"task_key":      taskKey,
		"status":        run.Status,
		"error_type":    et,
		"error_message": run.ErrorMessage,
		"severity":      severity,
		"likely_cause":  causes,
		"suggestions":   actions,
	}
}

// runView 精简 run 视图。
type runView struct {
	ID           uint
	TaskKey      string
	Status       string
	ErrorType    string
	ErrorMessage string
	Details      string
}

// planModeInput 请求制定执行计划。
type planModeInput struct {
	Goal    string   `json:"goal" jsonschema:"目标,如:修复 t1 反复失败"`
	Context []string `json:"context,omitempty" jsonschema:"已知线索,可选"`
}

// planMode 让助手先输出结构化执行计划(借鉴 zcode plan-mode):
// 步骤/依赖/是否需用户确认/预估影响,便于用户在执行前纠偏。
func (r *Registry) planMode(_ context.Context, in planModeInput) (string, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return errJSON("goal 必填", nil), nil
	}
	type step struct {
		Action string `json:"action"`
		NeedConfirm bool `json:"need_confirm"`
		Tool   string `json:"tool,omitempty"`
	}
	steps := []step{
		{Action: "收集现状:任务健康、最近执行、错误分类", Tool: "get_sync_health / diagnose_run"},
		{Action: "定位根因:比对成功与失败 run 的步骤链", Tool: "get_run_detail"},
		{Action: "给出修复建议与风险提示", NeedConfirm: false},
		{Action: "如需执行动作,向用户申请确认后调用危险工具", NeedConfirm: true},
	}
	return marshalJSON(map[string]any{
		"goal":         goal,
		"plan":         steps,
		"context":      in.Context,
		"note":         "按步骤执行;dangerous 工具必须等用户确认后再调用",
	}), nil
}
