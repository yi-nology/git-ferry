package tools

import (
	"context"
	"encoding/json"
	"strings"
)

// deepAnalyzeInput 深度分析(子代理风格:一次多步,减少来回)。
type deepAnalyzeInput struct {
	TaskKey  string `json:"task_key,omitempty" jsonschema:"聚焦某个任务;空则看全局失败"`
	Question string `json:"question,omitempty" jsonschema:"要回答的问题,如:为什么 t1 总是超时"`
}

// analyzeHit 一条失败摘要。
type analyzeHit struct {
	RunID    uint   `json:"run_id"`
	TaskKey  string `json:"task_key"`
	Status   string `json:"status"`
	ErrType  string `json:"error_type,omitempty"`
	ErrMsg   string `json:"error_message,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// deepAnalyze 串起健康评分+最近失败+诊断规则,输出结构化结论。
// 借鉴 zcode subagent(general-purpose):把多步查询压成一次工具调用,
// 降低模型轮次与上下文消耗。
func (r *Registry) deepAnalyze(ctx context.Context, in deepAnalyzeInput) (string, error) {
	tasks, _, err := r.svc.ListTasks(ctx, in.TaskKey, 0, pageLimit)
	if err != nil {
		return errJSON("查询任务失败", err), nil
	}

	failures := []analyzeHit{}
	recent := []map[string]any{}

	for _, t := range tasks {
		runs, _, herr := r.svc.ListHistory(ctx, t.Key, 0, 5)
		if herr != nil || len(runs) == 0 {
			continue
		}
		last := runs[0]
		recent = append(recent, map[string]any{
			"task_key": t.Key,
			"status":   last.Status,
			"error":    truncateStr(last.ErrorMessage, 120),
		})
		if last.Status != "failed" {
			continue
		}
		v := &runView{
			ID: last.ID, TaskKey: t.Key, Status: last.Status,
			ErrorType: last.ErrorType, ErrorMessage: last.ErrorMessage,
			Details: last.Details,
		}
		d := diagnoseRunView(v, t.Key)
		failures = append(failures, analyzeHit{
			RunID:    last.ID,
			TaskKey:  t.Key,
			Status:   last.Status,
			ErrType:  last.ErrorType,
			ErrMsg:   truncateStr(last.ErrorMessage, 200),
			Severity: fmtStr(d["severity"]),
		})
	}

	return marshalJSON(map[string]any{
		"question":    in.Question,
		"task_key":    in.TaskKey,
		"failures":    failures,
		"recent":      recent,
		"conclusion":  summarizeAnalyze(in.Question, failures),
		"playbook":    analyzeRules(),
		"next_action": nextActions(failures),
	}), nil
}

// analyzeRules 运维 playbook。
func analyzeRules() []map[string]string {
	return []map[string]string{
		{"when": "auth/401/403", "do": "检查 token 是否过期/权限是否含 repo push"},
		{"when": "network/timeout", "do": "检查内网出口、proxy、调大 default_timeout"},
		{"when": "divergent/conflict", "do": "比对目标独有提交,决定 merge 或关 keep_divergent"},
		{"when": "push rejected", "do": "检查分支保护;确认后可 git_force"},
		{"when": "lfs", "do": "安装 git-lfs 或关闭任务 git_lfs"},
		{"when": "bundle/backup", "do": "检查 backup_dir 可写与磁盘空间"},
	}
}

func summarizeAnalyze(question string, failures []analyzeHit) string {
	if len(failures) == 0 {
		return "最近没有失败记录;若 " + question + " 仍异常,建议 rebuild_task 后观察。"
	}
	types := map[string]int{}
	for _, f := range failures {
		types[f.ErrType]++
	}
	dominant := ""
	max := 0
	for k, v := range types {
		if v > max {
			dominant, max = k, v
		}
	}
	var b strings.Builder
	b.WriteString("共 " + itoa(len(failures)) + " 条失败;主因类型=" + dominant + "。")
	if question != "" {
		b.WriteString("针对问题「" + question + "」:优先按 playbook 中 " + dominant + " 条目处置。")
	}
	return b.String()
}

func nextActions(failures []analyzeHit) []string {
	if len(failures) == 0 {
		return []string{"无需处理"}
	}
	acts := []string{"diagnose_run(run_id=" + itoa(int(failures[0].RunID)) + ") 拿详细建议"}
	if failures[0].ErrType == "auth" {
		acts = append(acts, "更新平台 token 后 retry_sync_run")
	}
	if failures[0].ErrType == "divergent" || failures[0].ErrType == "conflict" {
		acts = append(acts, "get_run_detail 看目标独有提交")
	}
	acts = append(acts, "rebuild_task 全量重建(需确认)")
	return acts
}

func fmtStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// rebuildInput 全量重建。
type rebuildInput struct {
	TaskKey string `json:"task_key" jsonschema:"任务 key"`
	Reason  string `json:"reason,omitempty" jsonschema:"原因说明"`
}

// rebuildIn 危险:清 workdir 全量重拉(壳层 /ops/rebuild 同语义)。
func (r *Registry) rebuildIn(ctx context.Context, in rebuildInput) (string, error) {
	return r.dangerGuard(ctx, "rebuild_task", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in rebuildInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败"}), nil
		}
		// 触发一次同步即等价 rebuild(壳层会清 workdir);此处直接 RunTaskWithTrigger
		if err := r.svc.RunTaskWithTrigger(ctx, in.TaskKey, "ai_rebuild", nil); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": err.Error()}), nil
		}
		return marshalJSON(map[string]any{
			"status":   "ok",
			"message":  "已触发全量重建",
			"task_key": in.TaskKey,
		}), nil
	})
}
