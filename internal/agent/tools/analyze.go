package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/yi-nology/git-ferry-core/health"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
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
	weakDims := map[string]int{} // 维度 → 出现次数(score<60)
	now := time.Now()

	for _, t := range tasks {
		// 健康维度（与 get_sync_health 同源）
		facts := health.Facts{
			"has_name":          textutil.BoolFact(t.Name != ""),
			"has_cron":          textutil.BoolFact(t.Cron != ""),
			"cron":              t.Cron,
			"enabled":           textutil.BoolFact(t.Enabled),
			"keep_divergent":    textutil.BoolFact(t.KeepDivergent),
			"git_force":         textutil.BoolFact(t.GitForce),
			"force_push_policy": t.ForcePushPolicy,
			"backup_enabled":    textutil.BoolFact(t.GitBundle),
			"sync_wiki":         textutil.BoolFact(t.SyncWiki),
		}
		runs, _, herr := r.svc.ListHistory(ctx, t.Key, 0, 5)
		if herr == nil && len(runs) > 0 {
			snaps := make([]health.RunSnapshot, 0, len(runs))
			for _, rr := range runs {
				end := time.Time{}
				if rr.EndTime != nil {
					end = *rr.EndTime
				}
				snaps = append(snaps, health.RunSnapshot{
					Status: rr.Status, ErrorMessage: rr.ErrorMessage, EndAt: end,
				})
			}
			for k, v := range health.CollectRunFacts(snaps, now) {
				facts[k] = v
			}
		} else {
			facts["has_history"] = "false"
		}
		dimRes := health.EvaluateDimensions(nil, facts)
		for _, d := range dimRes.Dimensions {
			if d.Score < 60 {
				weakDims[d.Name]++
			}
		}
		recent = append(recent, map[string]any{
			"task_key":   t.Key,
			"health":     dimRes.Score,
			"level":      dimRes.Level,
			"weak_dims":  weakDimensionNames(dimRes.Dimensions),
			"dim_action": dimRes.Actions,
		})
		if herr == nil && len(runs) > 0 {
			last := runs[0]
			recent[len(recent)-1]["status"] = last.Status
			recent[len(recent)-1]["error"] = textutil.Truncate(last.ErrorMessage, 120)
			if last.Status == "failed" {
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
					ErrMsg:   textutil.Truncate(last.ErrorMessage, 200),
					Severity: fmtStr(d["severity"]),
				})
			}
		}
	}

	return marshalJSON(map[string]any{
		"question":    in.Question,
		"task_key":    in.TaskKey,
		"failures":    failures,
		"recent":      recent,
		"weak_dims":   rankWeakDims(weakDims),
		"conclusion":  summarizeAnalyze(in.Question, failures, weakDims),
		"playbook":    analyzeRules(),
		"next_action": nextActions(failures, weakDims),
	}), nil
}

func weakDimensionNames(dims []health.Dimension) []string {
	out := []string{}
	for _, d := range dims {
		if d.Score < 60 {
			out = append(out, d.Name)
		}
	}
	return out
}

func rankWeakDims(m map[string]int) []map[string]any {
	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		out = append(out, map[string]any{"dimension": x.k, "tasks": x.v})
	}
	return out
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

func summarizeAnalyze(question string, failures []analyzeHit, weakDims map[string]int) string {
	if len(failures) == 0 && len(weakDims) == 0 {
		return "最近没有失败记录且健康维度均正常;若 " + question + " 仍异常,建议 rebuild_task 后观察。"
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
	if len(failures) > 0 {
		b.WriteString("共 " + textutil.Itoa(len(failures)) + " 条失败;主因类型=" + dominant + "。")
	}
	if len(weakDims) > 0 {
		b.WriteString("薄弱维度:")
		for k, v := range weakDims {
			b.WriteString(" " + k + "×" + textutil.Itoa(v) + ";")
		}
	}
	if question != "" && dominant != "" {
		b.WriteString("针对问题「" + question + "」:优先按 playbook 中 " + dominant + " 条目处置。")
	}
	return b.String()
}

func nextActions(failures []analyzeHit, weakDims map[string]int) []string {
	acts := []string{}
	if len(failures) > 0 {
		acts = append(acts, "diagnose_run(run_id="+textutil.Itoa(int(failures[0].RunID))+") 拿详细建议")
		if failures[0].ErrType == "auth" {
			acts = append(acts, "更新平台 token 后 retry_sync_run")
		}
		if failures[0].ErrType == "divergent" || failures[0].ErrType == "conflict" {
			acts = append(acts, "get_run_detail 看目标独有提交")
		}
		acts = append(acts, "rebuild_task 全量重建(需确认)")
	}
	// 维度驱动的动作
	if weakDims["reliability"] > 0 {
		acts = append(acts, "对 reliability 低的任务跑 history +diagnose")
	}
	if weakDims["freshness"] > 0 {
		acts = append(acts, "检查停摆任务的 cron/enabled")
	}
	if weakDims["safety"] > 0 {
		acts = append(acts, "复核 force_push_policy / 漂移(ops +drift)")
	}
	if weakDims["schedule"] > 0 {
		acts = append(acts, "补 cron 或启用任务 task +update")
	}
	if len(acts) == 0 {
		return []string{"无需处理"}
	}
	return acts
}

func fmtStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
