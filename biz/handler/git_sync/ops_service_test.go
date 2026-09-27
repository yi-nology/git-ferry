package git_sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/health"
)

func TestScoreLevelMapping(t *testing.T) {
	// 经 health DSL,满分应 gold
	cfg := health.DefaultConfig()
	res := cfg.Evaluate(health.Facts{
		"has_name": "true", "has_cron": "true",
		"has_history": "true", "recent_success": "true", "error_free": "true",
	})
	require.Equal(t, "gold", res.Level)
}

func TestCSVEscape(t *testing.T) {
	assert.Equal(t, "plain", csvEscape("plain"))
	assert.Equal(t, `"a,b"`, csvEscape("a,b"))
	assert.Equal(t, `"say ""hi"""`, csvEscape(`say "hi"`))
}

func TestBoolFact(t *testing.T) {
	assert.Equal(t, "true", boolFact(true))
	assert.Equal(t, "false", boolFact(false))
}

func TestMatchToFilter(t *testing.T) {
	f := matchToFilter(map[string][]string{
		"include_globs": {"team-*"},
		"exclude":       {"team-x"},
	})
	require.NotNil(t, f)
	assert.True(t, f.Allow("team-a", "A"))
	assert.False(t, f.Allow("team-x", "X"))
}
