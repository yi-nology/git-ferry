package git_sync

import (
	"net/http"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"

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
	assert.Equal(t, "plain", textutil.CSVEscape("plain"))
	assert.Equal(t, `"a,b"`, textutil.CSVEscape("a,b"))
	assert.Equal(t, `"say ""hi"""`, textutil.CSVEscape(`say "hi"`))
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

func TestGenerateDeployKey(t *testing.T) {
	h := hertzserver.Default()
	h.POST("/api/v1/ops/deploy-key", GenerateDeployKey)
	body := `{"comment":"mirror-a"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/deploy-key",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusOK, w.Code)
	resp := w.Body.String()
	assert.Contains(t, resp, "private_key_pem")
	assert.Contains(t, resp, "public_key")
	assert.Contains(t, resp, "SHA256:")
}
