package git_sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/githubapi"
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
	if strategy != "preserve" && strategy != "single" && strategy != "flat" && strategy != "mixed" {
		response.BadRequest(c, "strategy must be preserve|single|flat|mixed")
		return
	}
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

	// 1) 导入源平台仓库（可选）
	if req.ImportNew != nil && *req.ImportNew {
		_, _ = svc.SyncPlatformReposFiltered(ctx, req.SourcePlatform, &corebridge.RepoImportFilter{
			ExcludeArchived: true,
		})
	}

	// 2) 过滤出源 org 下的仓库
	all, err := svc.ListReposByPlatform(ctx, req.SourcePlatform)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	orgLower := strings.ToLower(req.SourceOrg)
	result := &ops.OrgMirrorResp{
		Strategy:   strategy,
		SourceOrg:  req.SourceOrg,
		Target:     req.GetTargetOrg(),
		DryRun:     dryRun,
		Items:      []*ops.OrgMirrorItem{},
		Warnings:   []string{},
	}
	if result.Target == "" {
		result.Target = req.GetTargetUser()
	}

	for _, r := range all {
		if !strings.EqualFold(r.PlatformOwner, req.SourceOrg) {
			continue
		}
		_ = orgLower
		srcName := r.PlatformOwner + "/" + r.PlatformRepo
		tOwner, tRepo := resolveOrgTarget(strategy, r.PlatformOwner, r.PlatformRepo, req.GetTargetOrg(), req.GetTargetUser())
		tName := tOwner + "/" + tRepo
		item := &ops.OrgMirrorItem{Source: srcName, Target: tName}
		result.Planned++

		if dryRun {
			item.Action = "planned"
			result.Items = append(result.Items, item)
			continue
		}

		// 3) 目标仓登记（按 clone URL；已存在则跳过创建）
		tURL := rewriteRepoURL(r.CloneURL, tOwner, tRepo)
		if tURL == "" {
			tURL = rewriteRepoURL(dstPlat.InstanceURL, tOwner, tRepo)
		}
		tRepoRec, terr := svc.CreateRepo(ctx, &corebridge.CreateRepoRequest{
			Name:        tName,
			RemoteURL:   tURL,
			PlatformID:  dstPlat.ID,
			AccessToken: r.AccessToken,
		})
		if terr != nil {
			// 已存在等错误：尝试按名称找
			item.Action = "skipped"
			item.Message = terr.Error()
			result.Items = append(result.Items, item)
			result.Warnings = append(result.Warnings, tName+": "+terr.Error())
			continue
		}
		result.Imported++

		if !createTasks {
			item.Action = "imported"
			result.Items = append(result.Items, item)
			continue
		}

		// 4) 建同步任务
		srcKey := r.Key
		if srcKey == "" {
			item.Action = "failed"
			item.Message = "source repo key empty"
			result.Items = append(result.Items, item)
			continue
		}
		taskName := "org-mirror-" + tRepo
		_, terr = svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
			Name:          taskName,
			SourceRepoKey: srcKey,
			SourceBranch:  "*",
			TargetRepoKey: tRepoRec.Key,
			TargetBranch:  "*",
			SyncMode:      "all",
			GitTags:       true,
			IncludeBranches: "",
		})
		if terr != nil {
			item.Action = "failed"
			item.Message = terr.Error()
			result.Warnings = append(result.Warnings, taskName+": "+terr.Error())
		} else {
			item.Action = "task_created"
			result.TasksCreated++
		}
		result.Items = append(result.Items, item)
	}

	if !dryRun {
		recordAudit(ctx, c, "org_mirror", "platform", req.SourceOrg,
			fmt.Sprintf("org 映射 %s→%s strategy=%s planned=%d imported=%d tasks=%d",
				req.SourceOrg, result.Target, strategy, result.Planned, result.Imported, result.TasksCreated))
	} else {
		result.Warnings = append(result.Warnings, "dry_run=true：未写入")
	}
	response.Success(c, result)
}

// resolveOrgTarget 目标 owner/repo 映射（复用 internal/orgmap 语义）。
func resolveOrgTarget(strategy, sourceOwner, sourceRepo, targetOrg, targetUser string) (owner, repo string) {
	repo = sourceRepo
	switch strategy {
	case "single":
		if targetOrg != "" {
			return targetOrg, repo
		}
		return sourceOwner, repo
	case "flat":
		if targetUser != "" {
			return targetUser, repo
		}
		return sourceOwner, repo
	case "mixed":
		if targetUser != "" && strings.EqualFold(sourceOwner, targetUser) {
			return targetUser, repo
		}
		if targetOrg != "" {
			return targetOrg, repo
		}
		return sourceOwner, repo
	default:
		return sourceOwner, repo
	}
}

// rewriteRepoURL 把 clone URL 中的 owner/repo 换成目标（支持 https 与 ssh）。
func rewriteRepoURL(raw, owner, repo string) string {
	if raw == "" || owner == "" || repo == "" {
		return raw
	}
	// https://host/old/oldrepo.git
	if i := strings.Index(raw, "://"); i > 0 {
		rest := raw[i+3:]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return raw
		}
		prefix := raw[:i+3+slash+1]
		return prefix + owner + "/" + repo + ".git"
	}
	// git@host:old/oldrepo.git
	if i := strings.Index(raw, ":"); i > 0 && strings.Contains(raw[:i], "@") {
		return raw[:i+1] + owner + "/" + repo + ".git"
	}
	return raw
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
	plat, err := svc.GetPlatform(ctx, req.PlatformKey)
	if err != nil || plat == nil {
		response.NotFound(c, "platform not found")
		return
	}
	if !githubapi.IsGitHub(plat.Type) {
		response.BadRequest(c, "starred import currently supports github only")
		return
	}
	max := int(req.Max)
	if max <= 0 {
		max = 100
	}
	starred, err := githubapi.ListStarred(ctx, plat.APIURL, plat.AccessToken, max)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	resp := &ops.ImportListResp{
		Source:   "starred:" + req.PlatformKey,
		DryRun:   dryRun,
		Found:    int32(len(starred)),
		Items:    []*ops.RepoImportPreview{},
		Warnings: []string{},
	}
	for i := range starred {
		s := &starred[i]
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
