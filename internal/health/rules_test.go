package health

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_DefaultGold(t *testing.T) {
	cfg := DefaultConfig()
	res := cfg.Evaluate(Facts{
		"has_name":       "true",
		"has_cron":       "true",
		"has_history":    "true",
		"recent_success": "true",
		"error_free":     "true",
	})
	require.Equal(t, 100, res.Score)
	assert.Equal(t, "gold", res.Level)
	assert.Empty(t, res.Issues)
}

func TestEvaluate_BasicWhenEmpty(t *testing.T) {
	cfg := DefaultConfig()
	res := cfg.Evaluate(Facts{})
	assert.Equal(t, 0, res.Score)
	assert.Equal(t, "basic", res.Level)
	assert.NotEmpty(t, res.Issues)
}

func TestEvaluate_CustomWeights(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{ID: "a", Attr: "has_name", Op: "truthy", Weight: 1},
			{ID: "b", Attr: "has_cron", Op: "truthy", Weight: 1},
		},
		Levels: []Level{{Name: "gold", MinScore: 50}, {Name: "basic", MinScore: 0}},
	}
	res := cfg.Evaluate(Facts{"has_name": "true"})
	assert.Equal(t, 50, res.Score)
	assert.Equal(t, "gold", res.Level)
}

func TestFilter_IncludeExclude(t *testing.T) {
	f := &Filter{IncludeGlobs: []string{"team-*"}, Exclude: []string{"team-secret"}}
	assert.True(t, f.Allow("team-a", "Team A"))
	assert.False(t, f.Allow("team-secret", "Secret"))
	assert.False(t, f.Allow("other", "Other")) // 不在 include

	// 无 include 时 exclude 生效
	f2 := &Filter{ExcludeGlobs: []string{"*-tmp"}}
	assert.True(t, f2.Allow("app", "App"))
	assert.False(t, f2.Allow("app-tmp", "App Tmp"))
}
