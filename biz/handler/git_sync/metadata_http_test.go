package git_sync

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// HTTP 层断言：Bind/状态码/对外文案契约（引擎逻辑迁至 core，测试在
// git-ferry-core/service/metadata_*_test.go）。

func TestMetadataBackup_MissingRepoKey(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/metadata-backup", MetadataBackup)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/metadata-backup", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "repo_key")
}

func TestMetadataBackup_UnknownRepo(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/metadata-backup", MetadataBackup)
	body := `{"repo_key":"nope"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/metadata-backup",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "repo not found")
}

func TestMetadataRestore_MissingRepoKey(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/metadata-restore", MetadataRestore)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/metadata-restore", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "repo_key")
}

func TestMetadataRestore_RequiresBackupDir(t *testing.T) {
	// 测试 Service 未配置 backup_dir：校验顺序上 backup_dir 先于快照/repo，
	// 必须回 400 + 契约文案（core ErrMetadataValidation → 壳 validation 分支）。
	setupOpsHTTP(t)
	h := opsEngine()
	h.POST("/api/v1/ops/metadata-restore", MetadataRestore)
	body := `{"repo_key":"nope"}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ops/metadata-restore",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "sync.backup_dir not configured")
}

func TestListMetadataBackups_OK(t *testing.T) {
	setupOpsHTTP(t)
	h := opsEngine()
	h.GET("/api/v1/ops/metadata-backups", ListMetadataBackups)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ops/metadata-backups", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "items")
	assert.Contains(t, w.Body.String(), `"total":0`)
}

func TestMetadataVerifyMap(t *testing.T) {
	// 响应结构与历史 sampleMetadataVerify 逐键一致
	reason := metadataVerifyMap(corebridge.VerifyReport{Reason: "backup_dir not configured"})
	assert.Equal(t, map[string]any{"ok": false, "reason": "backup_dir not configured"}, reason)

	ok := metadataVerifyMap(corebridge.VerifyReport{Checked: 2, OK: true, Warnings: []string{}})
	assert.Equal(t, map[string]any{"checked": 2, "ok": true, "warnings": []string{}}, ok)

	bad := metadataVerifyMap(corebridge.VerifyReport{Checked: 1, OK: false, Warnings: []string{"x: issues.json missing"}})
	assert.Equal(t, false, bad["ok"])
	assert.Equal(t, []string{"x: issues.json missing"}, bad["warnings"])
}
