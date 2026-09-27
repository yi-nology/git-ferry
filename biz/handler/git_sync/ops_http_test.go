package git_sync

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/tpl"
)

func setupOpsHTTP(t *testing.T) {
	t.Helper()
	// 复用 TestMain 建好的 Service(未 Start,避免 cron 访问空表);
	// 只注入独立模板库,避免用例间串扰。
	st, err := tpl.Open(filepath.Join(t.TempDir(), "templates.json"))
	require.NoError(t, err)
	SetTplStore(st)
}

func opsEngine() *hertzserver.Hertz {
	h := hertzserver.Default()
	h.GET("/api/v1/ops/overview", SyncOverview)
	h.GET("/api/v1/ops/health-score", HealthScore)
	h.GET("/api/v1/ops/inventory", RepoInventory)
	h.GET("/api/v1/ops/templates", ListTemplates)
	h.POST("/api/v1/ops/templates", CreateTemplate)
	h.POST("/api/v1/ops/templates/delete", DeleteTemplate)
	h.POST("/api/v1/ops/templates/apply", ApplyTemplate)
	h.POST("/api/v1/ops/retry", RetryRun)
	h.POST("/api/v1/ops/retry-batch", BatchRetryFailed)
	return h
}

func TestOpsOverview_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/overview", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "repo_count")
}

func TestOpsHealthScore_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/health-score?limit=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "items")
}

func TestOpsInventory_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/inventory", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "orphan_repos")
}

func TestTemplates_CRUD(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()

	// create
	body := `{"name":"nightly","spec":{"cron":"0 2 * * *"},"tags":["backup"]}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.ID)
	assert.Equal(t, "nightly", created.Data.Name)

	// list
	w = ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/templates", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "nightly")

	// apply dry-run
	applyBody := `{"template_id":"` + created.Data.ID + `","dry_run":true}`
	w = ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates/apply",
		&ut.Body{Body: strings.NewReader(applyBody), Len: len(applyBody)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "dry_run")

	// delete
	w = ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates/delete?id="+created.Data.ID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// get after delete via list
	w = ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/templates", nil)
	assert.NotContains(t, w.Body.String(), `"nightly"`)
}

func TestCreateTemplate_MissingName(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	body := `{"spec":{"cron":"* * * * *"}}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRetryRun_MissingRunID(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	body := `{}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/retry",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "run_id")
}

func TestRetryRun_NotFound(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	body := `{"run_id":99999}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/retry",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBatchRetry_Empty(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	body := `{"limit":5}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/retry-batch",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "retried")
}

func TestExportIssues_MissingRepo(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/issues-export", ExportIssues)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/issues-export", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
