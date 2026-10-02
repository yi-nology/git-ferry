package git_sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// MetadataBackup POST /api/v1/ops/metadata-backup
// 抓取 issues/PR/labels/milestones/releases 元数据快照,
// 可选下载 source archive / GitHub Release 附件 / Gists。
// 请求体绑定 biz/model/ops(IDL 生成,snake_case 标签)。
// 采集引擎在 core（Service.BackupMetadata，经 ProviderForPlatform 取数）；
// 本层只做 Bind → core → DTO 转换 → 审计。
func MetadataBackup(ctx context.Context, c *app.RequestContext) {
	var req ops.MetadataBackupReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.BackupMetadata(ctx, corebridge.MetadataBackupOptions{
		RepoKey:         req.RepoKey,
		MaxIssues:       int(req.MaxItems),
		MaxPRs:          int(req.MaxItems),
		MaxReleases:     int(req.MaxItems),
		IncludeIssues:   optBoolDefault(req.WithIssues),
		IncludePRs:      optBoolDefault(req.WithPRs),
		IncludeReleases: optBoolDefault(req.WithReleases),
		IncludeSource:   optBoolDefault(req.WithArchives),
		IncludeAssets:   optBoolDefault(req.WithAssets),
		IncludeGists:    optBoolDefault(req.WithGists),
		Since:           req.GetSince(),
	})
	if err != nil {
		metadataEngineError(c, err)
		return
	}

	recordAudit(ctx, c, "metadata_backup", "backup", req.RepoKey,
		fmt.Sprintf("元数据快照 issues=%d prs=%d releases=%d archives=%d assets=%d gists=%d",
			res.Counts["issues"], res.Counts["pull_requests"], res.Counts["releases"],
			res.Counts["archives"], res.Counts["release_assets"], res.Counts["gists"]))
	response.Success(c, metadataSnapshotToOps(&res.MetadataSnapshot))
}

// ListMetadataBackups GET /api/v1/ops/metadata-backups?repo_key=
func ListMetadataBackups(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	items, err := svc.ListMetadataBackups(ctx, c.Query("repo_key"))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	out := make([]*ops.MetadataSnapshot, 0, len(items))
	for i := range items {
		out = append(out, metadataSnapshotToOps(&items[i]))
	}
	response.Success(c, &ops.ListMetadataBackupsResp{Items: out, Total: int64(len(out))})
}

// metadataEngineError 元数据引擎错误 → HTTP：repo/平台缺失回 404、快照与
// backup_dir 校验回 400（文案逐字契约），其余（含 provider 构造/MkdirAll，
// 即使底层是 SDK ProviderError）一律 500 —— 与历史 newIssueProvider 分支一致，
// 详情只进服务端日志。显式 errors.Is 而非 Classify，避免 provider 4xx 被分类成 4xx。
func metadataEngineError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, corebridge.ErrRepoNotFound),
		errors.Is(err, corebridge.ErrPlatformNotFound),
		errors.Is(err, corebridge.ErrTargetPlatformNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, corebridge.ErrMetadataValidation):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}

// metadataSnapshotToOps core 快照 → IDL 生成的 ops DTO（字段一一对应）。
func metadataSnapshotToOps(s *corebridge.MetadataSnapshot) *ops.MetadataSnapshot {
	return &ops.MetadataSnapshot{
		RepoKey:   s.RepoKey,
		Platform:  s.Platform,
		Owner:     s.Owner,
		Repo:      s.Repo,
		CreatedAt: s.CreatedAt,
		Dir:       s.Dir,
		Counts:    s.Counts,
		Files:     s.Files,
		Archives:  s.Archives,
		Assets:    s.Assets,
		Warnings:  s.Warnings,
	}
}
