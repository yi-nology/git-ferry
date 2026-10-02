package git_sync

import (
	"context"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// OrgMirror POST /api/v1/ops/org-mirror
// 按策略（preserve|single|flat|mixed）把源组织仓库映射到目标命名空间，并可选建同步任务。
func OrgMirror(ctx context.Context, c *app.RequestContext) {
	var req ops.OrgMirrorReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.SourcePlatform == "" || req.SourceOrg == "" || req.TargetPlatform == "" {
		response.BadRequest(c, "source_platform, source_org, target_platform are required")
		return
	}
	strategy := req.GetStrategy()
	if strategy == "" {
		strategy = "preserve"
	}
	parsed, err := corebridge.ParseOrgMapStrategy(strategy)
	if err != nil {
		response.BadRequest(c, "strategy must be preserve|single|flat|mixed")
		return
	}
	strategy = string(parsed)
	dryRun := optBoolDefault(req.DryRun)
	createTasks := req.CreateTasks != nil && *req.CreateTasks

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	srcPlat, err := svc.GetPlatform(ctx, req.SourcePlatform)
	if err != nil || srcPlat == nil {
		response.NotFound(c, "source platform not found")
		return
	}
	dstPlat, err := svc.GetPlatform(ctx, req.TargetPlatform)
	if err != nil || dstPlat == nil {
		response.NotFound(c, "target platform not found")
		return
	}

	// 编排（过滤/映射/建仓/建任务/统计）在 core；壳保留校验、审计与响应包装。
	bulk, err := svc.BulkMirrorOrg(ctx, corebridge.BulkMirrorRequest{
		SourcePlatformKey: req.SourcePlatform,
		SourceOrg:         req.SourceOrg,
		TargetPlatform:    dstPlat,
		Strategy:          corebridge.OrgMapStrategy(strategy),
		TargetOrg:         req.GetTargetOrg(),
		TargetUser:        req.GetTargetUser(),
		ImportNew:         req.ImportNew != nil && *req.ImportNew,
		CreateTasks:       createTasks,
		DryRun:            dryRun,
	})
	if err != nil {
		response.FromError(c, err)
		return
	}

	result := &ops.OrgMirrorResp{
		Strategy:  strategy,
		SourceOrg: req.SourceOrg,
		Target:    req.GetTargetOrg(),
		DryRun:    dryRun,
		Items:     []*ops.OrgMirrorItem{},
		Warnings:  []string{},
	}
	if result.Target == "" {
		result.Target = req.GetTargetUser()
	}
	result.Planned = int32(bulk.Planned)
	result.Imported = int32(bulk.Imported)
	result.TasksCreated = int32(bulk.TasksCreated)
	for _, it := range bulk.Items {
		result.Items = append(result.Items, &ops.OrgMirrorItem{
			Source: it.Source, Target: it.Target, Action: it.Action, Message: it.Message,
		})
	}
	result.Warnings = append(result.Warnings, bulk.Warnings...)

	if !dryRun {
		recordAudit(ctx, c, "org_mirror", "platform", req.SourceOrg,
			fmt.Sprintf("org 映射 %s→%s strategy=%s planned=%d imported=%d tasks=%d",
				req.SourceOrg, result.Target, strategy, result.Planned, result.Imported, result.TasksCreated))
	} else {
		result.Warnings = append(result.Warnings, "dry_run=true：未写入")
	}
	response.Success(c, result)
}

// ImportStarred POST /api/v1/ops/import-starred
// 拉取平台 starred 仓库并可选导入。
func ImportStarred(ctx context.Context, c *app.RequestContext) {
	var req ops.ImportStarredReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.PlatformKey == "" {
		response.BadRequest(c, "platform_key is required")
		return
	}
	dryRun := optBoolDefault(req.DryRun)
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	max := int(req.Max)
	if max <= 0 {
		max = 100
	}
	// 列举（平台校验 + 能力门控 + 分页）在 core；壳做 DTO 映射与可选导入。
	starred, err := svc.ListStarredRepos(ctx, req.PlatformKey, max)
	if err != nil {
		response.FromError(c, err)
		return
	}
	resp := &ops.ImportListResp{
		Source:   "starred:" + req.PlatformKey,
		DryRun:   dryRun,
		Found:    int32(len(starred)),
		Items:    []*ops.RepoImportPreview{},
		Warnings: []string{},
	}
	for _, s := range starred {
		resp.Items = append(resp.Items, &ops.RepoImportPreview{
			FullName: s.FullName, CloneUrl: s.CloneURL,
			Fork: s.Fork, Archived: s.Archived, Stars: int32(s.Stars),
		})
	}
	if !dryRun && req.ImportNew != nil && *req.ImportNew {
		n, ierr := svc.SyncPlatformReposFiltered(ctx, req.PlatformKey, &corebridge.RepoImportFilter{})
		if ierr != nil {
			resp.Warnings = append(resp.Warnings, ierr.Error())
		}
		resp.Imported = int32(n)
	}
	response.Success(c, resp)
}
