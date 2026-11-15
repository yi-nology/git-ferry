package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// MetadataMigrateReq 元数据迁移请求(POST /api/v1/ops/metadata-migrate)。
type MetadataMigrateReq struct {
	TaskKey  string `json:"task_key" form:"task_key"`
	Issues   bool   `json:"issues" form:"issues"`
	Comments bool   `json:"comments" form:"comments"`
	PRs      bool   `json:"prs" form:"prs"`
	DryRun   bool   `json:"dry_run" form:"dry_run"`
	Limit    int    `json:"limit" form:"limit"`
}

// MetadataMigrate 元数据迁移(Issue/PR 抽取+写入,SDK 2B Ensure* 幂等收敛)。
// 写操作走 WriteGuard;dry_run=true 只盘点不写(报告将创建/收敛数)。
func MetadataMigrate(ctx context.Context, c *app.RequestContext) {
	var req MetadataMigrateReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.TaskKey == "" {
		response.BadRequest(c, "task_key is required")
		return
	}
	if !req.Issues && !req.PRs {
		response.BadRequest(c, "select at least one of: issues / prs")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	rep, err := svc.MigrateRepoMetadata(ctx, corebridge.MetadataMigrateOptions{
		TaskKey:  req.TaskKey,
		Issues:   req.Issues,
		Comments: req.Comments,
		PRs:      req.PRs,
		DryRun:   req.DryRun,
		Limit:    req.Limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rep)
}
