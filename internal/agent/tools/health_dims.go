package tools

import "github.com/yi-nology/git-ferry-core/health"

// issuesFromDims / actionsFromDims:core v0.46.0 删除 EvalResult 顶层
// Issues/Actions 兼容投影后的本地派生(口径与 core issuesFromDimensions/
// actionsFromDimensions 一致:score<60 → "name: reason";score<70 且
// 非空 action 去重保序)——归因单源到 Dimensions,这里是展示层复刻。
func issuesFromDims(dims []health.Dimension) []string {
	var out []string
	for _, d := range dims {
		if d.Score < 60 && d.Reason != "" {
			out = append(out, d.Name+": "+d.Reason)
		}
	}
	return out
}

func actionsFromDims(dims []health.Dimension) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range dims {
		if d.Score < 70 && d.Action != "" && !seen[d.Action] {
			seen[d.Action] = true
			out = append(out, d.Action)
		}
	}
	return out
}
