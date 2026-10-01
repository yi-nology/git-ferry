package health

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteActions_Reliability(t *testing.T) {
	dims := []Dimension{
		{Name: "reliability", Score: 20, Reason: "连续失败 3 次"},
		{Name: "freshness", Score: 90},
	}
	acts := RouteActions(TaskBrief{TaskKey: "t1", TaskName: "T1", RunID: 42}, dims)
	require.NotEmpty(t, acts)
	// 优先级 1 应是 diagnose
	assert.Equal(t, 1, acts[0].Priority)
	assert.Contains(t, acts[0].Command, "+diagnose")
	assert.Contains(t, acts[0].Command, "42")

	// 含重试（danger）
	foundDanger := false
	for _, a := range acts {
		if a.Danger {
			foundDanger = true
		}
	}
	assert.True(t, foundDanger)
}

func TestRouteActions_SkipHealthy(t *testing.T) {
	dims := []Dimension{
		{Name: "reliability", Score: 95},
		{Name: "freshness", Score: 95},
		{Name: "safety", Score: 95},
	}
	acts := RouteActions(TaskBrief{TaskKey: "t1"}, dims)
	assert.Empty(t, acts)
}

func TestRouteActions_DriftAndOrphan(t *testing.T) {
	acts := RouteActions(TaskBrief{
		TaskKey: "t1", DriftN: 2, Orphan: true, RPOBreach: true,
	}, []Dimension{{Name: "safety", Score: 35, Reason: "drift=2"}})
	joined := ""
	for _, a := range acts {
		joined += a.Command + " "
	}
	assert.Contains(t, joined, "+drift")
	assert.Contains(t, joined, "+rpo")
	assert.Contains(t, joined, "+create")
}

func TestRouteActions_DedupeAndCap(t *testing.T) {
	dims := []Dimension{
		{Name: "reliability", Score: 10, Reason: "a"},
		{Name: "freshness", Score: 10, Reason: "b"},
		{Name: "schedule", Score: 10, Reason: "c"},
		{Name: "completeness", Score: 10, Reason: "d"},
	}
	acts := RouteActions(TaskBrief{TaskKey: "t1", RunID: 1}, dims)
	assert.LessOrEqual(t, len(acts), 8)
	// 优先级非递减
	for i := 1; i < len(acts); i++ {
		assert.LessOrEqual(t, acts[i-1].Priority, acts[i].Priority)
	}
}

func TestWeakDimensionNames(t *testing.T) {
	got := WeakDimensionNames([]Dimension{
		{Name: "a", Score: 50},
		{Name: "b", Score: 80},
	})
	assert.Equal(t, []string{"a"}, got)
}
