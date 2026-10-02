package git_sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// orgBulkEngine 只挂本切片的两个编排端点(与 opsEngine 互不复用,避免路由串扰)。
func orgBulkEngine() *hertzserver.Hertz {
	h := hertzserver.Default()
	h.POST("/api/v1/ops/import-public-org", ImportPublicOrg)
	h.POST("/api/v1/ops/org-mirror", OrgMirror)
	return h
}

// postData POST 后解析外层信封,返回 (状态码, message, data 原文)。
// message 是 400/404 的对外契约文案,与 data 一起逐字锁定。
func postData(t *testing.T, h *hertzserver.Hertz, path, body string) (int, string, string) {
	t.Helper()
	w := ut.PerformRequest(h.Engine, http.MethodPost, path,
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body=%s", w.Body.String())
	if env.Data == nil {
		env.Data = []byte("null")
	}
	return w.Code, env.Message, string(env.Data)
}

func createBulkPlatform(t *testing.T, p *corebridge.Platform) *corebridge.Platform {
	t.Helper()
	require.NoError(t, GetSyncService().CreatePlatform(context.Background(), p))
	return p
}

// TestImportPublicOrg_DryRun_ResponseShape 固定 dry_run 预览响应的逐字 JSON:
// 非 github 平台不触发远端列仓,found/items 恒为空、warnings 只有 dry_run 提示。
func TestImportPublicOrg_DryRun_ResponseShape(t *testing.T) {
	setupOpsHTTP(t)
	createBulkPlatform(t, &corebridge.Platform{
		Key: "gl-bulkimport", Name: "GL", Type: "gitlab", APIURL: "https://gitlab.example.com/api/v4",
	})

	h := orgBulkEngine()
	code, _, data := postData(t, h, "/api/v1/ops/import-public-org",
		`{"platform_key":"gl-bulkimport","org":"bulksrc","dry_run":true,"exclude_archived":true,"min_stars":3}`)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{
		"org": "bulksrc",
		"dry_run": true,
		"found": 0,
		"imported": 0,
		"created_tasks": 0,
		"items": [],
		"warnings": ["dry_run=true：未写入"],
		"filter": {
			"ExcludeArchived": true,
			"ExcludeForks": false,
			"MinStars": 3,
			"IncludeLanguage": "",
			"IncludeGlobs": null,
			"ExcludeGlobs": null
		}
	}`, data)
}

// TestImportPublicOrg_FullRun_ResponseShape github 平台走列仓预览 + 过滤导入,
// 固定 items/found/imported/warnings 的逐字 JSON(dry_run=false 才会真正导入)。
func TestImportPublicOrg_FullRun_ResponseShape(t *testing.T) {
	setupOpsHTTP(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`[{
				"id":1,"full_name":"bulksrc/pub1","name":"pub1",
				"owner":{"login":"bulksrc"},
				"clone_url":"https://github.com/bulksrc/pub1.git",
				"private":false,"fork":false,"stargazers_count":10,"default_branch":"main"
			}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	createBulkPlatform(t, &corebridge.Platform{
		Key: "gh-bulkimport", Name: "GH", Type: "github", APIURL: srv.URL + "/api/v3",
	})

	h := orgBulkEngine()
	code, _, data := postData(t, h, "/api/v1/ops/import-public-org",
		`{"platform_key":"gh-bulkimport","org":"bulksrc","create_tasks":true,"target_org":"backup","target_platform":"gl-bulkimport","dry_run":false}`)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{
		"org": "bulksrc",
		"dry_run": false,
		"found": 1,
		"imported": 1,
		"created_tasks": 0,
		"items": [{
			"full_name": "bulksrc/pub1",
			"clone_url": "https://github.com/bulksrc/pub1.git",
			"fork": false,
			"archived": false,
			"stars": 10
		}],
		"warnings": ["按平台凭证列取组织公开仓；大组织可能只拉到部分元数据"],
		"filter": {
			"ExcludeArchived": false,
			"ExcludeForks": false,
			"MinStars": 0,
			"IncludeLanguage": "",
			"IncludeGlobs": null,
			"ExcludeGlobs": null
		}
	}`, data)
}

// TestImportPublicOrg_MissingPlatform 平台不存在 → 404 + 逐字文案。
func TestImportPublicOrg_MissingPlatform(t *testing.T) {
	setupOpsHTTP(t)
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/import-public-org",
		`{"platform_key":"nope-bulk","org":"bulksrc","dry_run":true}`)
	require.Equal(t, http.StatusNotFound, code)
	require.Equal(t, "platform not found", msg)
}

// TestImportPublicOrg_MissingRequired 入参缺失 → 400 + 逐字文案。
func TestImportPublicOrg_MissingRequired(t *testing.T) {
	setupOpsHTTP(t)
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/import-public-org", `{"org":"  "}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "platform_key and org are required", msg)
}

// TestOrgMirror_DryRun_ResponseShape 固定 dry_run 映射预览的逐字 JSON。
func TestOrgMirror_DryRun_ResponseShape(t *testing.T) {
	setupOpsHTTP(t)
	src := createBulkPlatform(t, &corebridge.Platform{
		Key: "src-bulkmirror", Name: "SRC", Type: "gitlab", APIURL: "https://src.example.com/api/v4",
	})
	createBulkPlatform(t, &corebridge.Platform{
		Key: "dst-bulkmirror", Name: "DST", Type: "gitlab",
		APIURL: "https://dst.example.com/api/v4", InstanceURL: "https://dst.example.com",
	})
	svc := GetSyncService()
	_, err := svc.CreateRepo(context.Background(), &corebridge.CreateRepoRequest{
		Name: "bulkmirror/app", RemoteURL: "https://src.example.com/bulkmirror/app.git",
		PlatformID: src.ID,
	})
	require.NoError(t, err)

	h := orgBulkEngine()
	code, _, data := postData(t, h, "/api/v1/ops/org-mirror",
		`{"source_platform":"src-bulkmirror","source_org":"bulkmirror","target_platform":"dst-bulkmirror","target_org":"backup","strategy":"single","dry_run":true}`)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{
		"strategy": "single",
		"source_org": "bulkmirror",
		"target": "backup",
		"dry_run": true,
		"planned": 1,
		"imported": 0,
		"tasks_created": 0,
		"items": [{"source": "bulkmirror/app", "target": "backup/app", "action": "planned", "message": ""}],
		"warnings": ["dry_run=true：未写入"]
	}`, data)
}

// TestOrgMirror_CreateTasks_ResponseShape 非 dry_run:登记目标仓 + 建任务,
// 固定 action=task_created 与三段计数的逐字 JSON。
func TestOrgMirror_CreateTasks_ResponseShape(t *testing.T) {
	setupOpsHTTP(t)
	src := createBulkPlatform(t, &corebridge.Platform{
		Key: "src-bulkmirror2", Name: "SRC", Type: "gitlab", APIURL: "https://src2.example.com/api/v4",
	})
	createBulkPlatform(t, &corebridge.Platform{
		Key: "dst-bulkmirror2", Name: "DST", Type: "gitlab",
		APIURL: "https://dst2.example.com/api/v4", InstanceURL: "https://dst2.example.com",
	})
	svc := GetSyncService()
	_, err := svc.CreateRepo(context.Background(), &corebridge.CreateRepoRequest{
		Name: "bulkmirror2/app", RemoteURL: "https://src2.example.com/bulkmirror2/app.git",
		PlatformID: src.ID,
	})
	require.NoError(t, err)

	h := orgBulkEngine()
	code, _, data := postData(t, h, "/api/v1/ops/org-mirror",
		`{"source_platform":"src-bulkmirror2","source_org":"bulkmirror2","target_platform":"dst-bulkmirror2","target_org":"backup","strategy":"single","create_tasks":true,"dry_run":false}`)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{
		"strategy": "single",
		"source_org": "bulkmirror2",
		"target": "backup",
		"dry_run": false,
		"planned": 1,
		"imported": 1,
		"tasks_created": 1,
		"items": [{"source": "bulkmirror2/app", "target": "backup/app", "action": "task_created", "message": ""}],
		"warnings": []
	}`, data)
}

// TestOrgMirror_BadStrategy 策略非法 → 400 + 逐字文案。
func TestOrgMirror_BadStrategy(t *testing.T) {
	setupOpsHTTP(t)
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/org-mirror",
		`{"source_platform":"a","source_org":"b","target_platform":"c","strategy":"nope"}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "strategy must be preserve|single|flat|mixed", msg)
}

// TestOrgMirror_MissingRequired 入参缺失 → 400 + 逐字文案。
func TestOrgMirror_MissingRequired(t *testing.T) {
	setupOpsHTTP(t)
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/org-mirror", `{"source_org":"b"}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "source_platform, source_org, target_platform are required", msg)
}

// TestOrgMirror_MissingSourcePlatform 源平台缺失 → 404 + 逐字文案。
func TestOrgMirror_MissingSourcePlatform(t *testing.T) {
	setupOpsHTTP(t)
	createBulkPlatform(t, &corebridge.Platform{
		Key: "dst-bulkmirror4", Name: "D", Type: "gitlab", APIURL: "https://dst4.example.com/api/v4",
	})
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/org-mirror",
		`{"source_platform":"nope-bulkmirror4","source_org":"x","target_platform":"dst-bulkmirror4"}`)
	require.Equal(t, http.StatusNotFound, code)
	require.Equal(t, "source platform not found", msg)
}

// TestOrgMirror_MissingTargetPlatform 目标平台缺失 → 404 + 逐字文案。
func TestOrgMirror_MissingTargetPlatform(t *testing.T) {
	setupOpsHTTP(t)
	src := createBulkPlatform(t, &corebridge.Platform{
		Key: "src-bulkmirror3", Name: "SRC", Type: "gitlab", APIURL: "https://src3.example.com/api/v4",
	})
	_ = src
	h := orgBulkEngine()
	code, msg, _ := postData(t, h, "/api/v1/ops/org-mirror",
		`{"source_platform":"src-bulkmirror3","source_org":"bulkmirror3","target_platform":"nope-bulkmirror3"}`)
	require.Equal(t, http.StatusNotFound, code)
	require.Equal(t, "target platform not found", msg)
}
