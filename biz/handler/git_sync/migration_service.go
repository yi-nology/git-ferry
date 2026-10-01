package git_sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// ExportGitHubMigration POST /api/v1/ops/migration
// 调 GitHub Migration API 导出用户/组织全量 tar.gz(仓库+issues+PR+releases)。
// 与逐项 issues-export 互补:这是平台原生「一键全量归档」。
func ExportGitHubMigration(ctx context.Context, c *app.RequestContext) {
	var req ops.ExportMigrationReq
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
	if plat.Type != corebridge.PlatformTypeGitHub {
		response.BadRequest(c, "migration export only supports github platforms")
		return
	}
	prov, perr := newIssueProvider(plat, "")
	if perr != nil {
		response.InternalError(c, perr.Error())
		return
	}
	if !prov.Capabilities().Migrations {
		response.BadRequest(c, "platform does not support migrations")
		return
	}
	mm := prov.(sdkprov.MigrationManager)

	// org=="" 用户级(/user/migrations),非空组织级(/orgs/{org}/migrations)。
	org := optStr(req.Org)
	info, err := mm.CreateMigration(ctx, org, sdkprov.CreateMigrationOptions{
		LockRepositories: true,
		ExcludeMetadata:  false,
	})
	if err != nil {
		response.InternalError(c, migrationCreateErrText(err))
		return
	}

	recordAudit(ctx, c, "export_migration", "platform", req.PlatformKey,
		fmt.Sprintf("发起 GitHub Migration 导出 org=%s", org))

	result := map[string]any{
		"migration_id": info.ID,
		"state":        info.State,
		"archive_url":  info.ArchiveURL,
		"note":         "GitHub 异步生成归档;稍后用 GET /user/migrations/{id} 查询,archive_url 可下载 tar.gz",
	}

	// wait=true 时轮询到 completed(最多 60s);轮询节奏留在壳层。
	if optBool(req.Wait) && info.ID != 0 {
		for i := 0; i < 12; i++ {
			time.Sleep(5 * time.Second)
			if done := pollMigration(ctx, mm, org, info.ID); done != nil {
				result["state"] = done.State
				result["archive_url"] = done.ArchiveURL
				break
			}
		}
	}

	response.Success(c, result)
}

// migrationCreateErrText 近似原手写文案 "github migration create failed: status %d":
// 平台错误带 HTTP status 时按原格式,否则透出平台错误串。
func migrationCreateErrText(err error) string {
	var pe *sdkprov.ProviderError
	if errors.As(err, &pe) && pe.StatusCode != 0 {
		return fmt.Sprintf("github migration create failed: status %d", pe.StatusCode)
	}
	return "github migration create failed: " + err.Error()
}

// pollMigration 查一次迁移状态;exported/failed 终态才返回(否则 nil 继续轮询)。
func pollMigration(ctx context.Context, mm sdkprov.MigrationManager, org string, id int64) *sdkprov.MigrationInfo {
	mi, err := mm.GetMigration(ctx, org, id)
	if err != nil {
		return nil
	}
	if mi.State == "exported" || mi.State == "failed" {
		return mi
	}
	return nil
}
