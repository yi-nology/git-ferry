package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectRunFacts_Empty(t *testing.T) {
	f := CollectRunFacts(nil, time.Now())
	assert.Equal(t, "false", f["has_history"])
	assert.Equal(t, "0", f["run_total"])
}

func TestCollectRunFacts_StreakAndRate(t *testing.T) {
	now := time.Now()
	runs := []RunSnapshot{
		{Status: "failed", ErrorMessage: "boom", EndAt: now.Add(-1 * time.Hour)},
		{Status: "failed", ErrorMessage: "boom", EndAt: now.Add(-2 * time.Hour)},
		{Status: "success", EndAt: now.Add(-3 * time.Hour)},
		{Status: "success", EndAt: now.Add(-4 * time.Hour)},
	}
	f := CollectRunFacts(runs, now)
	assert.Equal(t, "4", f["run_total"])
	assert.Equal(t, "2", f["run_success"])
	assert.Equal(t, "2", f["fail_streak"])
	assert.Equal(t, "false", f["recent_success"])
	assert.Equal(t, "false", f["error_free"])
	assert.Equal(t, "true", f["has_history"])
}

func TestEvaluateDimensions_AllGreen(t *testing.T) {
	f := Facts{
		"has_name": "true", "has_cron": "true", "cron": "0 2 * * *",
		"enabled": "true", "keep_divergent": "true", "force_push_policy": "block",
		"backup_enabled": "true", "sync_wiki": "true",
		"has_history": "true", "run_total": "10", "run_success": "10",
		"fail_streak": "0", "recent_success": "true", "last_run_age_hours": "1",
	}
	res := EvaluateDimensions(nil, f)
	require.NotEmpty(t, res.Dimensions)
	assert.GreaterOrEqual(t, res.Score, 90)
	assert.Equal(t, "gold", res.Level)
	assert.Empty(t, res.Actions)

	names := map[string]Dimension{}
	for _, d := range res.Dimensions {
		names[d.Name] = d
	}
	assert.Equal(t, 100, names["reliability"].Score)
	assert.Equal(t, 100, names["freshness"].Score)
	assert.Equal(t, 100, names["safety"].Score)
}

func TestEvaluateDimensions_FailureDragsScore(t *testing.T) {
	f := Facts{
		"has_name": "true", "has_cron": "false", "enabled": "true",
		"force_push_policy": "allow", "git_force": "true",
		"has_history": "true", "run_total": "5", "run_success": "1",
		"fail_streak": "4", "recent_success": "false",
		"last_run_age_hours": "200",
	}
	res := EvaluateDimensions(nil, f)
	assert.Less(t, res.Score, 50)
	assert.Equal(t, "basic", res.Level)
	assert.NotEmpty(t, res.Issues)
	assert.NotEmpty(t, res.Actions)

	var reliability, safety *Dimension
	for i := range res.Dimensions {
		switch res.Dimensions[i].Name {
		case "reliability":
			reliability = &res.Dimensions[i]
		case "safety":
			safety = &res.Dimensions[i]
		}
	}
	require.NotNil(t, reliability)
	require.NotNil(t, safety)
	assert.LessOrEqual(t, reliability.Score, 25) // 连续失败 4 次
	assert.Less(t, safety.Score, 70)             // allow + git_force
	assert.NotEmpty(t, safety.Action)
}

func TestEvaluateDimensions_NoHistory(t *testing.T) {
	res := EvaluateDimensions(nil, Facts{"has_name": "true", "has_cron": "true", "enabled": "true"})
	assert.Equal(t, "basic", res.Level)
	found := false
	for _, d := range res.Dimensions {
		if d.Name == "reliability" {
			found = true
			assert.Equal(t, 0, d.Score)
			assert.NotEmpty(t, d.Action)
		}
	}
	assert.True(t, found)
}

func TestDimensionWeightsSum(t *testing.T) {
	total := 0
	for _, s := range DefaultDimensions() {
		total += s.Weight
	}
	assert.Equal(t, 100, total)
}

func TestEvaluateDimensions_DriftPullsSafety(t *testing.T) {
	f := Facts{
		"has_name": "true", "has_cron": "true", "enabled": "true",
		"force_push_policy": "block",
		"has_history":       "true", "run_total": "5", "run_success": "5",
		"fail_streak": "0", "recent_success": "true", "last_run_age_hours": "1",
		"drift_count": "2",
	}
	res := EvaluateDimensions(nil, f)
	var safety *Dimension
	for i := range res.Dimensions {
		if res.Dimensions[i].Name == "safety" {
			safety = &res.Dimensions[i]
		}
	}
	require.NotNil(t, safety)
	assert.LessOrEqual(t, safety.Score, 35)
	assert.Contains(t, safety.Reason, "drift=2")
	assert.NotEmpty(t, safety.Action)
	// 其它维度仍可高分，但总分被 safety 拖低
	assert.Less(t, res.Score, 90)
}

func TestSortedDimensionNames(t *testing.T) {
	names := SortedDimensionNames()
	assert.ElementsMatch(t,
		[]string{"reliability", "freshness", "schedule", "safety", "completeness"},
		names,
	)
}
