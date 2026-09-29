package git_sync

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/githubapi"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// BackupGistsReq 平台级 gists 备份请求。
type BackupGistsReq struct {
	PlatformKey string `json:"platform_key" form:"platform_key" query:"platform_key"`
	MaxGists    int    `json:"max_gists" form:"max_gists"`
}

// BackupGists POST /api/v1/ops/gists-backup
// 备份平台 token 可见的 gists 到冷备目录(仅 GitHub)。
func BackupGists(ctx context.Context, c *app.RequestContext) {
	var req BackupGistsReq
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
	if !githubapi.IsGitHub(plat.Type) {
		response.BadRequest(c, "gists backup only supports GitHub platforms")
		return
	}
	backupDir := svc.BackupDir()
	if backupDir == "" {
		response.BadRequest(c, "sync.backup_dir not configured")
		return
	}
	dest := filepath.Join(backupDir, "gists", textutil.SanitizePathToken(req.PlatformKey),
		time.Now().UTC().Format("20060102-150405"))
	count, warnings, err := githubapi.BackupGists(ctx, plat.APIURL, plat.AccessToken, dest, req.MaxGists)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "gists_backup", "backup", req.PlatformKey,
		"备份 gists "+strconv.Itoa(count))
	response.Success(c, map[string]any{
		"platform_key": req.PlatformKey,
		"count":        count,
		"dir":          dest,
		"warnings":     warnings,
	})
}
