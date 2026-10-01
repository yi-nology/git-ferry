package health

import (
	"fmt"
	"sort"
	"strings"
)

// Action 建议动作：可直接执行的命令或人工步骤。
type Action struct {
	// Kind: cli | manual
	Kind string `json:"kind"`
	// Dimension 触发维度
	Dimension string `json:"dimension"`
	// Priority 越小越优先（1=紧急）
	Priority int    `json:"priority"`
	Title    string `json:"title"`
	// Command 可复制的 CLI（kind=cli）；<KEY>/<RUN_ID> 已尽量填好
	Command string `json:"command,omitempty"`
	// Danger 命令是否需 --yes / 用户确认
	Danger bool   `json:"danger,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// TaskBrief 路由输入：任务 + 维度结果 + 关联资源。
type TaskBrief struct {
	TaskKey   string
	TaskName  string
	RunID     int64 // 最近失败 run；0 表示无
	DriftN    int
	Orphan    bool
	RPOBreach bool
}

// RouteActions 把低分维度翻译成具体动作（去重、按优先级排序）。
func RouteActions(brief TaskBrief, dims []Dimension) []Action {
	out := []Action{}
	key := brief.TaskKey
	if key == "" {
		key = "<TASK>"
	}
	runRef := "<RUN_ID>"
	if brief.RunID > 0 {
		runRef = fmt.Sprintf("%d", brief.RunID)
	}

	for _, d := range dims {
		if d.Score >= 70 {
			continue // 良好不打扰
		}
		switch d.Name {
		case "reliability":
			p := 2
			if d.Score < 30 {
				p = 1
			}
			acts := []Action{{
				Kind: "cli", Dimension: d.Name, Priority: p,
				Title:   "诊断失败根因",
				Command: fmt.Sprintf("gitferry history +diagnose --run-id %s --format json", runRef),
				Reason:  d.Reason,
			}}
			if brief.RunID > 0 {
				acts = append(acts,
					Action{
						Kind: "cli", Dimension: d.Name, Priority: p + 1,
						Title: "查看执行步骤链",
						Command: fmt.Sprintf("gitferry history +detail --task %s --run-id %s --format json",
							key, runRef),
						Reason: d.Reason,
					},
					Action{
						Kind: "cli", Dimension: d.Name, Priority: p + 2,
						Title:   "重试一次（确认后）",
						Command: fmt.Sprintf("gitferry history +retry --run-id %s --yes", runRef),
						Danger:  true,
						Reason:  d.Reason,
					},
				)
			}
			out = append(out, acts...)
		case "freshness":
			out = append(out, Action{
				Kind: "cli", Dimension: d.Name, Priority: 2,
				Title:   "检查任务调度与启停",
				Command: fmt.Sprintf("gitferry task +info --key %s --format json", key),
				Reason:  d.Reason,
			}, Action{
				Kind: "cli", Dimension: d.Name, Priority: 3,
				Title:   "补 cron 并启用（确认后）",
				Command: fmt.Sprintf(`gitferry task +update --key %s --cron "0 2 * * *" --enabled --yes`, key),
				Danger:  true,
				Reason:  d.Reason,
			})
		case "schedule":
			out = append(out, Action{
				Kind: "cli", Dimension: d.Name, Priority: 2,
				Title:   "配置调度",
				Command: fmt.Sprintf(`gitferry task +update --key %s --cron "0 2 * * *" --enabled --yes`, key),
				Danger:  true,
				Reason:  d.Reason,
			})
		case "safety":
			if brief.DriftN > 0 {
				out = append(out, Action{
					Kind: "cli", Dimension: d.Name, Priority: 1,
					Title:   "确认分支漂移",
					Command: fmt.Sprintf("gitferry ops +drift --task %s --format json", key),
					Reason:  d.Reason,
				})
			}
			out = append(out, Action{
				Kind: "manual", Dimension: d.Name, Priority: 2,
				Title:   "复核 force_push_policy / keep_divergent",
				Command: fmt.Sprintf("gitferry task +info --key %s --format json", key),
				Reason:  d.Reason,
			})
		case "completeness":
			if strings.Contains(strings.Join(d.Detail, " "), "冷备") {
				out = append(out, Action{
					Kind: "manual", Dimension: d.Name, Priority: 3,
					Title:   "评估开启冷备 bundle",
					Command: fmt.Sprintf("gitferry task +info --key %s  # 编辑时开启 git_bundle", key),
					Reason:  d.Reason,
				})
			} else {
				out = append(out, Action{
					Kind: "manual", Dimension: d.Name, Priority: 3,
					Title:   "补全任务元数据",
					Command: fmt.Sprintf("gitferry task +info --key %s", key),
					Reason:  d.Reason,
				})
			}
		}
	}

	// 跨维度：孤儿 / RPO
	if brief.Orphan {
		out = append(out, Action{
			Kind: "cli", Dimension: "coverage", Priority: 2,
			Title:   "为孤儿仓库建同步任务",
			Command: "gitferry repo +list --format json && gitferry task +create --name ... --source-repo ... --target-repo ...",
			Reason:  "仓库无任务覆盖",
		})
	}
	if brief.RPOBreach {
		out = append(out, Action{
			Kind: "cli", Dimension: "freshness", Priority: 1,
			Title:   "检查冷备时效",
			Command: "gitferry ops +rpo --max-seconds 86400 --format json",
			Reason:  "RPO 超标",
		})
	}

	return dedupeActions(out)
}

func dedupeActions(in []Action) []Action {
	seen := map[string]bool{}
	out := make([]Action, 0, len(in))
	for _, a := range in {
		if a.Title == "" {
			continue
		}
		k := a.Dimension + "|" + a.Title + "|" + a.Command
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Title < out[j].Title
	})
	// 全局最多 8 条，避免刷屏
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// WeakDimensionNames 列出低分维度名。
func WeakDimensionNames(dims []Dimension) []string {
	out := []string{}
	for _, d := range dims {
		if d.Score < 60 {
			out = append(out, d.Name)
		}
	}
	return out
}
