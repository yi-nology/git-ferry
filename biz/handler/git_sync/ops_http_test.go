package git_sync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
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

func TestRebuild_MissingTaskKey(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/rebuild", RebuildRepo)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/rebuild",
		&ut.Body{Body: strings.NewReader(`{}`), Len: 2},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRebuild_TaskNotFound(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/rebuild", RebuildRepo)
	body := `{"task_key":"nope"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/rebuild",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSyncPlatformFiltered_MissingKey(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/sync-platform", SyncPlatformFiltered)
	body := `{"exclude_archived":true}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/sync-platform",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExportMigration_NotFoundPlatform(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/migration", ExportGitHubMigration)
	body := `{"platform_key":"nope"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/migration",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExportMigration_NotGitHub(t *testing.T) {
	setupOpsHTTP(t)
	// 先建一个非 github 平台
	svc := GetSyncService()
	p := &corebridge.Platform{
		Key: "gitlab-x", Name: "GL", Type: "gitlab",
		APIURL: "https://gitlab.com/api/v4",
	}
	require.NoError(t, svc.CreatePlatform(context.Background(), p))

	h := opsEngine()
	h.POST("/api/v1/ops/migration", ExportGitHubMigration)
	body := `{"platform_key":"gitlab-x"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/migration",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "github")
}

func TestAuditReport_CSV(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/audit-report", AuditReport)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/audit-report?format=csv&limit=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, string(w.Header().Get("Content-Type")), "text/csv")
}

func TestSafeWorkDir(t *testing.T) {
	ok, err := safeWorkDir("/tmp/gf", "task-1")
	require.NoError(t, err)
	assert.Contains(t, ok, "task-1")

	for _, bad := range []string{"", "..", "a/b", `a\b`, "../etc", "a/../.."} {
		_, err := safeWorkDir("/tmp/gf", bad)
		require.Error(t, err, "expected reject for %q", bad)
	}
}

func TestDiagnose_MissingRunID(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/diagnose", DiagnoseRun)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/diagnose", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDiagnoseRun_RuleOutput(t *testing.T) {
	run := &corebridge.SyncRun{Status: "failed", ErrorType: "auth", ErrorMessage: "401 token"}
	d := diagnoseRun(run)
	assert.Equal(t, "critical", d["severity"])
	assert.NotEmpty(t, d["likely_cause"])
	assert.NotEmpty(t, d["suggestions"])
}

func TestBundles_ListEmpty(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/bundles", ListBundles)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/bundles", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "items")
}

func TestBundles_VerifyMissing(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/bundles/verify", VerifyBundle)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/bundles/verify?name=../../etc/passwd", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func opsEngineFull() *hertzserver.Hertz {
	h := opsEngine()
	h.GET("/api/v1/ops/rpo", RPOReport)
	h.GET("/api/v1/ops/backup-manifest/verify", VerifyBackupManifest)
	h.POST("/api/v1/ops/backup-manifest", BuildBackupManifest)
	h.GET("/api/v1/ops/dr-drill/history", DrillHistory)
	h.GET("/api/v1/ops/dr-drill/chain/verify", VerifyDrillChain)
	h.GET("/api/v1/ops/audit-chain/verify", VerifyAuditChain)
	h.GET("/api/v1/ops/rbac", GetRBAC)
	h.POST("/api/v1/ops/drift", DetectDrift)
	h.POST("/api/v1/ops/backup-cleanup", CleanupBackups)
	return h
}

func TestRPO_RequiresBackupDir(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/rpo", nil)
	// 未配置 backup_dir 时返回 500/400,但不能 panic
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError || w.Code == http.StatusBadRequest)
}

func TestDrillHistory_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/dr-drill/history?limit=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "items")
}

func TestAuditChainVerify_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/audit-chain/verify", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "checked")
}

func TestRBAC_DefaultReadonly(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	// 未走鉴权中间件时角色缺省 readonly
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/rbac", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "readonly")
}

func TestBackupCleanup_RequiresConfirm(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/backup-cleanup", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDrift_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/drift", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "checked")
}

func TestOIDCJWT_ParsesValidToken(t *testing.T) {
	// 构造 HS256 JWT
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice","role":"operator","exp":4102444800}`))
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := header + "." + payload + "." + sig

	claims, err := parseHS256JWT(token, "secret")
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.Sub)
	assert.Equal(t, "operator", stringClaim(claims.Extra, "role"))

	_, err = parseHS256JWT(token, "wrong")
	assert.Error(t, err)
}

func TestParseRole(t *testing.T) {
	assert.Equal(t, RoleAdmin, ParseRole("admin"))
	assert.Equal(t, RoleOperator, ParseRole("Operator"))
	assert.Equal(t, RoleReadonly, ParseRole("nope"))
}

func TestExportDrillHistory_InvalidFormat(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	h.GET("/api/v1/ops/dr-drill/export", ExportDrillHistory)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/dr-drill/export?format=xml", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExportDrillHistory_CSV(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngineFull()
	h.GET("/api/v1/ops/dr-drill/export", ExportDrillHistory)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/dr-drill/export?format=csv", nil)
	// 未配置 backup_dir 时 500;配置后应 200 + csv
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError, "code=%d", w.Code)
}

func TestOpsTodo_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/todo", OpsTodo)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/todo", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "items")
	assert.Contains(t, body, "by_kind")
	assert.Contains(t, body, "generated_at")
}

func TestOpsHealthScore_HasDimensions(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/health-score?limit=10", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// 空任务集时仍有 summary/attention 结构
	assert.Contains(t, body, "summary")
	assert.Contains(t, body, "attention")
}

func TestTemplates_ExtendsChain(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine() // 已含 templates create/apply

	reqBody := `{"name":"base","spec":{"cron":"0 1 * * *"}}`
	w1 := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates",
		&ut.Body{Body: strings.NewReader(reqBody), Len: len(reqBody)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusCreated, w1.Code)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &created))
	baseID := created.Data.ID
	require.NotEmpty(t, baseID)

	childBody := `{"name":"child","extends":"` + baseID + `","spec":{"cron":"0 5 * * *"}}`
	w2 := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates",
		&ut.Body{Body: strings.NewReader(childBody), Len: len(childBody)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusCreated, w2.Code)
	var child struct {
		Data struct {
			ID      string `json:"id"`
			Extends string `json:"extends"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &child))
	assert.Equal(t, baseID, child.Data.Extends)

	applyBody := `{"template_id":"` + child.Data.ID + `","dry_run":true}`
	w3 := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/templates/apply",
		&ut.Body{Body: strings.NewReader(applyBody), Len: len(applyBody)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusOK, w3.Code)
	out := w3.Body.String()
	assert.Contains(t, out, "extends_chain")
	assert.Contains(t, out, "effective_spec")
}

func TestOpsTodo_EmptyIsOK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/todo", OpsTodo)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/todo", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items  []any          `json:"items"`
			Total  int            `json:"total"`
			ByKind map[string]int `json:"by_kind"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body.Data.Items)
	assert.GreaterOrEqual(t, body.Data.Total, 0)
	assert.NotNil(t, body.Data.ByKind)
}

func TestHealthScore_EmptyAttentionShape(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/health-score", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items       []map[string]any `json:"items"`
			Attention   []map[string]any `json:"attention"`
			Summary     map[string]any   `json:"summary"`
			GeneratedAt string           `json:"generated_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body.Data.Attention)
	assert.NotNil(t, body.Data.Summary)
	assert.NotEmpty(t, body.Data.GeneratedAt)
}

func TestResolveOrgTarget_HTTP(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/resolve-org-target", ResolveOrgTarget)

	body := `{"source_repo_key":"github/acme/app","org_mapping":"single","target_org":"backup","target_platform":"gitlab"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/resolve-org-target",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusOK, w.Code)
	out := w.Body.String()
	assert.Contains(t, out, "backup")
	assert.Contains(t, out, "target_key")
}

func TestResolveOrgTarget_BadPolicy(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/resolve-org-target", ResolveOrgTarget)
	body := `{"source_repo_key":"a/b","org_mapping":"nope"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/resolve-org-target",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRepoFiles_MissingTaskKey(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/repo-files", RepoFiles)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/repo-files", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPushBackup_MissingFields(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/push-backup", PushBackup)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/push-backup",
		&ut.Body{Body: strings.NewReader(`{"task_key":"t1"}`), Len: len(`{"task_key":"t1"}`)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPushBackup_DryRun(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/push-backup", PushBackup)
	body := `{"task_key":"t1","remote":"https://example.com/a/b.git","dry_run":true}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/push-backup",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	// 无 workdir 或 task 不存在时 400/404；存在则 200 dry_run
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound,
		"code=%d body=%s", w.Code, w.Body.String())
}
