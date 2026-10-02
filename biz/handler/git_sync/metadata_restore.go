package git_sync

import (
	"context"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// MetadataRestore POST /api/v1/ops/metadata-restore
// 将 metadata-backup 快照回灌到目标仓：labels → milestones → issues → PRs(以 issue 形态) → releases。
// dry_run 缺省 true（安全默认）；overwrite=false 时同名跳过。
// 回灌引擎在 core（Service.RestoreMetadata）；本层只做 Bind → core → DTO 转换 → 审计。
func MetadataRestore(ctx context.Context, c *app.RequestContext) {
	var req ops.MetadataRestoreReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}
	dryRun := optBoolDefault(req.DryRun) // nil → true
	overwrite := req.IsSetOverwrite() && req.GetOverwrite()

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.RestoreMetadata(ctx, corebridge.RestoreRequest{
		RepoKey:        req.RepoKey,
		SnapshotDir:    req.GetSnapshotDir(),
		Kinds:          req.GetKinds(),
		DryRun:         dryRun,
		Overwrite:      overwrite,
		TargetPlatform: req.GetTargetPlatform(),
		TargetOwner:    req.GetTargetOwner(),
		TargetRepo:     req.GetTargetRepo(),
	})
	if err != nil {
		metadataEngineError(c, err)
		return
	}

	if !dryRun {
		recordAudit(ctx, c, "metadata_restore", "backup", req.RepoKey,
			fmt.Sprintf("元数据回灌 target=%s dry_run=false", res.Target))
	}
	response.Success(c, restoreResultToOps(res))
}

// restoreResultToOps core 回灌结果 → IDL 生成的 ops DTO（字段一一对应；
// Applied/Skipped/Failed/Details 为 core 扩展视图，不进 HTTP 响应）。
func restoreResultToOps(r *corebridge.RestoreResult) *ops.MetadataRestoreResult {
	stats := make(map[string]*ops.RestoreKindStat, len(r.Stats))
	for k, v := range r.Stats {
		stats[k] = &ops.RestoreKindStat{
			Planned: int32(v.Planned),
			Created: int32(v.Created),
			Skipped: int32(v.Skipped),
			Failed:  int32(v.Failed),
		}
	}
	return &ops.MetadataRestoreResult{
		SnapshotDir: r.SnapshotDir,
		Target:      r.Target,
		Stats:       stats,
		Warnings:    r.Warnings,
		DryRun:      r.DryRun,
		StartedAt:   r.StartedAt,
		FinishedAt:  r.FinishedAt,
	}
}
