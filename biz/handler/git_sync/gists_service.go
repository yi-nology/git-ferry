package git_sync

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// BackupGists POST /api/v1/ops/gists-backup
// 备份平台 token 可见的 gists 到冷备目录(仅 GitHub)。
func BackupGists(ctx context.Context, c *app.RequestContext) {
	var req ops.BackupGistsReq
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
	prov, perr := newIssueProvider(plat, "")
	if perr != nil {
		response.InternalError(c, perr.Error())
		return
	}
	count, warnings, err := svc.BackupGists(ctx, prov, dest, int(req.MaxGists))
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
