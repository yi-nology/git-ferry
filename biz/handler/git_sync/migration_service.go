package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ExportMigrationReq GitHub Migration API 全量归档(借鉴 gitbackup createUserMigration)。
type ExportMigrationReq struct {
	// PlatformKey 必须是 github 类型平台
	PlatformKey string `json:"platform_key" form:"platform_key" query:"platform_key"`
	// Org 组织名(与 User 二选一;都空=当前 token 用户)
	Org string `json:"org" form:"org" query:"org"`
	// ArchiveURL 是否只返回归档下载地址
	Wait bool `json:"wait" form:"wait" query:"wait"`
}

// ExportGitHubMigration POST /api/v1/ops/migration
// 调 GitHub Migration API 导出用户/组织全量 tar.gz(仓库+issues+PR+releases)。
// 与逐项 issues-export 互补:这是平台原生「一键全量归档」。
func ExportGitHubMigration(ctx context.Context, c *app.RequestContext) {
	var req ExportMigrationReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.PlatformKey == "" {
		response.BadRequest(c, "platform_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	plat, err := svc.GetPlatform(ctx, req.PlatformKey)
	if err != nil || plat == nil {
		response.NotFound(c, "platform not found")
		return
	}
	if !strings.EqualFold(plat.Type, "github") {
		response.BadRequest(c, "migration export only supports github platforms")
		return
	}

	token := plat.AccessToken
	if token == "" {
		response.InternalError(c, "platform has no access token")
		return
	}
	apiBase := strings.TrimRight(plat.APIURL, "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

	var migURL string
	if req.Org != "" {
		migURL = apiBase + "/orgs/" + req.Org + "/migrations"
	} else {
		migURL = apiBase + "/user/migrations"
	}

	body, _ := json.Marshal(map[string]any{
		"lock_repositories": true,
		"exclude_metadata":  false,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, migURL, strings.NewReader(string(body)))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		response.InternalError(c, fmt.Sprintf("github migration create failed: status %d", resp.StatusCode))
		return
	}

	var parsed struct {
		ID        int64  `json:"id"`
		State     string `json:"state"`
		ArchiveURL string `json:"archive_url"`
	}
	_ = json.Unmarshal(raw, &parsed)

	recordAudit(ctx, c, "export_migration", "platform", req.PlatformKey,
		fmt.Sprintf("发起 GitHub Migration 导出 org=%s", req.Org))

	result := map[string]any{
		"migration_id": parsed.ID,
		"state":        parsed.State,
		"archive_url":  parsed.ArchiveURL,
		"note":         "GitHub 异步生成归档;稍后用 GET /user/migrations/{id} 查询,archive_url 可下载 tar.gz",
	}

	// wait=true 时轮询到 completed(最多 60s)
	if req.Wait && parsed.ID != 0 {
		for i := 0; i < 12; i++ {
			time.Sleep(5 * time.Second)
			if done := pollMigration(ctx, apiBase, token, req.Org, parsed.ID); done != nil {
				result["state"] = done.State
				result["archive_url"] = done.ArchiveURL
				break
			}
		}
	}

	response.Success(c, result)
}

type migrationState struct {
	State      string `json:"state"`
	ArchiveURL string `json:"archive_url"`
}

func pollMigration(ctx context.Context, apiBase, token, org string, id int64) *migrationState {
	u := fmt.Sprintf("%s/user/migrations/%d", apiBase, id)
	if org != "" {
		u = fmt.Sprintf("%s/orgs/%s/migrations/%d", apiBase, org, id)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var st migrationState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil
	}
	if st.State == "exported" || st.State == "failed" {
		return &st
	}
	return nil
}

var _ = corebridge.PlatformStatusActive
var _ = consts.StatusOK
