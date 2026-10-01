package health

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Dimension 单项健康维度（Scorecards 模式）。
type Dimension struct {
	Name   string   `json:"name"`
	Weight int      `json:"weight"`
	Score  int      `json:"score"` // 0-100
	Reason string   `json:"reason"`
	Detail []string `json:"detail,omitempty"`
	Action string   `json:"action,omitempty"` // 建议动作（CLI/人工）
}

// EvalResult 聚合结果：总分 + 维度 + 兼容字段。
type EvalResult struct {
	Score      int         `json:"score"`
	Level      string      `json:"level"`
	Issues     []string    `json:"issues,omitempty"`
	Dimensions []Dimension `json:"dimensions,omitempty"`
	Actions    []string    `json:"actions,omitempty"`
}

// DimensionSpec 维度定义：从 Facts 取子集打分。
type DimensionSpec struct {
	Name   string
	Weight int
	// Evaluate 返回 0-100 分与 reason；detail/action 由 spec 填。
	Evaluate func(f Facts) (score int, reason string, detail []string, action string)
}

// DefaultDimensions 覆盖可靠性 / 新鲜度 / 调度 / 安全 / 完备性。
func DefaultDimensions() []DimensionSpec {
	return []DimensionSpec{
		{
			Name:   "reliability",
			Weight: 35,
			Evaluate: func(f Facts) (int, string, []string, string) {
				total := atoi(f["run_total"])
				ok := atoi(f["run_success"])
				failStreak := atoi(f["fail_streak"])
				lastOK := truthy(f["recent_success"])
				// 新信号：步骤级失败 / 重试堆积
				stepFails := atoi(f["step_fail_count"])
				retryHeavy := atoi(f["retry_total"]) >= 3

				if total == 0 {
					return 0, "尚无执行历史", nil, "gitferry task +run --key <TASK> --yes 做一次冒烟"
				}
				rate := ok * 100 / total
				reason := fmt.Sprintf("近 %d 次成功 %d 次（%d%%）", total, ok, rate)
				var detail []string
				if failStreak > 0 {
					detail = append(detail, fmt.Sprintf("连续失败 %d 次", failStreak))
				}
				if stepFails > 0 {
					detail = append(detail, fmt.Sprintf("步骤级失败 %d 处（fetch/push 等）", stepFails))
				}
				if retryHeavy {
					detail = append(detail, "重试次数偏高")
				}
				score := rate
				if !lastOK {
					score = minInt(score, 40)
				}
				if failStreak >= 3 {
					score = minInt(score, 25)
				}
				// 步骤失败额外扣分（说明链路有具体断点）
				if stepFails > 0 {
					score = minInt(score, 55)
				}
				if retryHeavy {
					score -= 10
				}
				action := ""
				if score < 60 {
					action = "gitferry history +diagnose --run-id <ID> 定位根因"
				}
				return clampScore(score), reason, detail, action
			},
		},
		{
			Name:   "freshness",
			Weight: 20,
			Evaluate: func(f Facts) (int, string, []string, string) {
				if !truthy(f["has_history"]) {
					return 0, "从未执行", nil, "配置 cron 或手动触发一次"
				}
				ageH := toFloat(f["last_run_age_hours"])
				reason := fmt.Sprintf("最近执行 %.1f 小时前", ageH)
				var score int
				switch {
				case ageH <= 26: // 覆盖日级任务
					score = 100
				case ageH <= 72:
					score = 70
				case ageH <= 168:
					score = 40
				default:
					score = 10
				}
				action := ""
				if score < 50 {
					action = "检查 cron 是否停用：gitferry task +info --key <TASK>"
				}
				return score, reason, nil, action
			},
		},
		{
			Name:   "schedule",
			Weight: 15,
			Evaluate: func(f Facts) (int, string, []string, string) {
				hasCron := truthy(f["has_cron"])
				enabled := truthy(f["enabled"])
				var detail []string
				score := 0
				if hasCron {
					score += 70
				} else {
					detail = append(detail, "未配置 cron")
				}
				if enabled {
					score += 30
				} else {
					detail = append(detail, "任务已停用")
				}
				reason := "调度配置"
				if hasCron && enabled {
					reason = "cron=" + f["cron"] + " 且已启用"
				}
				action := ""
				if !hasCron || !enabled {
					action = "gitferry task +update --key <TASK> --cron \"0 2 * * *\" --enabled"
				}
				return clampScore(score), reason, detail, action
			},
		},
		{
			Name:   "safety",
			Weight: 15,
			Evaluate: func(f Facts) (int, string, []string, string) {
				policy := f["force_push_policy"] // allow|block|backup_on_demand|""
				keepDiv := truthy(f["keep_divergent"])
				var detail []string
				score := 100
				switch policy {
				case "block":
					// 最安全
				case "backup_on_demand":
					score = 85
					detail = append(detail, "force 策略为 backup_on_demand")
				case "allow":
					score = 40
					detail = append(detail, "force 策略为 allow，可能覆盖目标历史")
				default:
					// 空：按 keep_divergent 兼容
					if !keepDiv {
						score = 50
						detail = append(detail, "keep_divergent=false 且无 force_push_policy")
					} else {
						score = 90
						detail = append(detail, "keep_divergent=true（默认安全）")
					}
				}
				if truthy(f["git_force"]) && score > 60 {
					score = 60
					detail = append(detail, "开启了 git_force")
				}
				// 实测漂移：目标/本地 refs 不一致 → 静默数据风险
				if driftN := atoi(f["drift_count"]); driftN > 0 {
					score = minInt(score, 35)
					detail = append(detail, fmt.Sprintf("检测到 %d 处分支漂移", driftN))
				} else if truthy(f["has_drift"]) {
					score = minInt(score, 35)
					detail = append(detail, "存在分支漂移")
				}
				reason := "强制推送保护"
				if policy != "" {
					reason = "force_push_policy=" + policy
				}
				if driftN := atoi(f["drift_count"]); driftN > 0 {
					reason += fmt.Sprintf("；drift=%d", driftN)
				}
				action := ""
				if score < 70 {
					action = "评估是否改为 block/backup_on_demand：gitferry task +update --key <TASK>"
				}
				if driftN := atoi(f["drift_count"]); driftN > 0 || truthy(f["has_drift"]) {
					action = "gitferry ops +drift --task <TASK> 确认分歧后决定 force/rebuild"
				}
				return score, reason, detail, action
			},
		},
		{
			Name:   "completeness",
			Weight: 15,
			Evaluate: func(f Facts) (int, string, []string, string) {
				var detail []string
				score := 0
				if truthy(f["has_name"]) {
					score += 30
				} else {
					detail = append(detail, "缺少任务名称")
				}
				if truthy(f["has_history"]) {
					score += 30
				}
				if truthy(f["backup_enabled"]) {
					score += 25
				} else {
					detail = append(detail, "未开启冷备 bundle")
				}
				if truthy(f["sync_wiki"]) {
					score += 15
				} else {
					detail = append(detail, "未同步 wiki")
				}
				action := ""
				if score < 50 {
					action = "补全任务元数据 / 考虑开启 --git-bundle"
				}
				return clampScore(score), "元数据与备份完备性", detail, action
			},
		},
	}
}

// EvaluateDimensions 按维度规格评分并聚合。
func EvaluateDimensions(specs []DimensionSpec, f Facts) EvalResult {
	if len(specs) == 0 {
		specs = DefaultDimensions()
	}
	dims := make([]Dimension, 0, len(specs))
	totalW, weighted := 0, 0
	var issues, actions []string

	for _, s := range specs {
		w := maxInt(s.Weight, 0)
		totalW += w
		score, reason := 0, ""
		var detail []string
		action := ""
		if s.Evaluate != nil {
			score, reason, detail, action = s.Evaluate(f)
		}
		score = clampScore(score)
		weighted += score * w
		d := Dimension{
			Name: s.Name, Weight: w, Score: score,
			Reason: reason, Detail: detail, Action: action,
		}
		dims = append(dims, d)
		if score < 60 {
			issues = append(issues, fmt.Sprintf("%s: %s", s.Name, reason))
		}
		if action != "" && score < 70 {
			actions = append(actions, action)
		}
	}

	overall := 0
	if totalW > 0 {
		overall = weighted / totalW
	}
	// 动作去重保序
	actions = uniqueKeepOrder(actions)
	return EvalResult{
		Score:      clampScore(overall),
		Level:      levelFromScore(overall),
		Issues:     issues,
		Dimensions: dims,
		Actions:    actions,
	}
}

func levelFromScore(score int) string {
	switch {
	case score >= 80:
		return "gold"
	case score >= 60:
		return "silver"
	case score >= 40:
		return "bronze"
	default:
		return "basic"
	}
}

// ---- facts 采集（纯函数，便于测试）----

// RunSnapshot 历史采集输入（由 handler 从 SyncRun 映射）。
type RunSnapshot struct {
	Status       string
	ErrorMessage string
	EndAt        time.Time // 零值表示未知
	// StepFails 本次执行中失败步骤数（fetch/push 等）
	StepFails int
	// RetryTotal 本次执行重试次数
	RetryTotal int
	// ErrorType 错误分类（auth/network/divergent/...）
	ErrorType string
}

// CollectRunFacts 从最近 N 次执行采 reliability/freshness 事实。
func CollectRunFacts(runs []RunSnapshot, now time.Time) Facts {
	f := Facts{
		"run_total":      "0",
		"run_success":    "0",
		"fail_streak":    "0",
		"has_history":    "false",
		"recent_success": "false",
		"error_free":     "true",
	}
	if len(runs) == 0 {
		return f
	}
	f["has_history"] = "true"
	f["run_total"] = strconv.Itoa(len(runs))

	ok := 0
	stepFails := 0
	retrySum := 0
	errTypes := map[string]int{}
	for _, r := range runs {
		if r.Status == "success" {
			ok++
		}
		stepFails += maxInt(r.StepFails, 0)
		retrySum += maxInt(r.RetryTotal, 0)
		if r.ErrorType != "" {
			errTypes[r.ErrorType]++
		}
	}
	f["run_success"] = strconv.Itoa(ok)
	f["step_fail_count"] = strconv.Itoa(stepFails)
	f["retry_total"] = strconv.Itoa(retrySum)
	if len(errTypes) > 0 {
		// 主导错误类型
		dom, n := "", 0
		for k, v := range errTypes {
			if v > n || (v == n && k < dom) {
				dom, n = k, v
			}
		}
		f["dominant_error"] = dom
	}

	// 连续失败（从最新往旧数）
	streak := 0
	for _, r := range runs {
		if r.Status == "success" {
			break
		}
		streak++
	}
	f["fail_streak"] = strconv.Itoa(streak)

	// 最新一条
	last := runs[0]
	f["recent_success"] = strconv.FormatBool(last.Status == "success")
	f["error_free"] = strconv.FormatBool(last.ErrorMessage == "")
	if !last.EndAt.IsZero() {
		hours := now.Sub(last.EndAt).Hours()
		if hours < 0 {
			hours = 0
		}
		f["last_run_age_hours"] = strconv.FormatFloat(hours, 'f', 2, 64)
	}
	return f
}

func truthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s != "" && s != "false" && s != "0"
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func clampScore(s int) int {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func uniqueKeepOrder(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// SortedDimensionNames 返回默认维度名（稳定顺序，供文档/测试）。
func SortedDimensionNames() []string {
	specs := DefaultDimensions()
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}
