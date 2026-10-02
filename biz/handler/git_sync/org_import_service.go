package git_sync

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ResolveOrgTarget POST /api/v1/ops/resolve-org-target
// 按 org_mapping 策略计算目标仓库 key（纯预览，不落库）。
func ResolveOrgTarget(ctx context.Context, c *app.RequestContext) {
	var req struct {
		SourceRepoKey  string `json:"source_repo_key"`
		OrgMapping     string `json:"org_mapping"`
		TargetOrg      string `json:"target_org"`
		TargetPlatform string `json:"target_platform"`
		IsPersonal     bool   `json:"is_personal"`
	}
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.SourceRepoKey == "" {
		response.BadRequest(c, "source_repo_key is required")
		return
	}
	policy, err := corebridge.ParseOrgMapStrategy(req.OrgMapping)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 导入侧 single/flat 共用 target_org 作为落点，缺落点报错，mixed 组织仓保持源 owner。
	out, err := corebridge.ResolveOrgTarget("", "", &corebridge.OrgMapOptions{
		Strategy:         policy,
		TargetOrg:        req.TargetOrg,
		TargetUser:       req.TargetOrg,
		SourceIsPersonal: req.IsPersonal,
		RequireTarget:    true,
		SourceKey:        req.SourceRepoKey,
		TargetPlatform:   req.TargetPlatform,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"policy":       string(policy),
		"target_owner": out.Owner,
		"target_repo":  out.Repo,
		"target_key":   out.Key,
	})
}

// ImportPublicOrgReq 导入公共 Org 公开仓（匿名/只读镜像）。
type ImportPublicOrgReq struct {
	PlatformKey     string `json:"platform_key"`
	Org             string `json:"org"`
	CreateTasks     bool   `json:"create_tasks"`
	TargetPlatform  string `json:"target_platform"`
	TargetOrg       string `json:"target_org"`
	ExcludeArchived *bool  `json:"exclude_archived"`
	ExcludeForks    *bool  `json:"exclude_forks"`
	MinStars        int32  `json:"min_stars"`
	DryRun          *bool  `json:"dry_run"`
	Max             int32  `json:"max"`
}

// ImportPublicOrg POST /api/v1/ops/import-public-org
// 列出组织公开仓并可选建同步任务（默认只导入仓库记录）。
// 编排在 core Service.ImportPublicOrg；壳做绑定、平台校验、审计与响应包装。
func ImportPublicOrg(ctx context.Context, c *app.RequestContext) {
	var req ImportPublicOrgReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.PlatformKey == "" || strings.TrimSpace(req.Org) == "" {
		response.BadRequest(c, "platform_key and org are required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	plat, perr := svc.GetPlatform(ctx, req.PlatformKey)
	if perr != nil || plat == nil {
		response.NotFound(c, "platform not found")
		return
	}

	dryRun := optBoolDefault(req.DryRun)
	max := int(req.Max)
	if max <= 0 {
		max = 100
	}
	filter := &corebridge.RepoImportFilter{
		ExcludeArchived: optBool(req.ExcludeArchived),
		ExcludeForks:    optBool(req.ExcludeForks),
		MinStars:        int(req.MinStars),
	}

	res, err := svc.ImportPublicOrg(ctx, corebridge.PublicOrgImportRequest{
		PlatformKey:    req.PlatformKey,
		Org:            req.Org,
		CreateTasks:    req.CreateTasks,
		TargetPlatform: req.TargetPlatform,
		TargetOrg:      req.TargetOrg,
		Filter:         filter,
		DryRun:         dryRun,
		Max:            max,
	})
	if err != nil {
		response.FromError(c, err)
		return
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, map[string]any{
			"full_name": it.FullName, "clone_url": it.CloneURL,
			"fork": it.Fork, "archived": it.Archived, "stars": it.Stars,
		})
	}
	warnings := append([]string{}, res.Warnings...)
	if !dryRun {
		recordAudit(ctx, c, "import_public_org", "platform", req.PlatformKey,
			"导入公共组织 "+req.Org+" 仓库数="+strconv.Itoa(res.Imported)+" 任务="+strconv.Itoa(res.CreatedTasks))
	} else {
		warnings = append(warnings, "dry_run=true：未写入")
	}
	response.Success(c, map[string]any{
		"org":           req.Org,
		"dry_run":       dryRun,
		"found":         len(items),
		"imported":      res.Imported,
		"created_tasks": res.CreatedTasks,
		"items":         items,
		"warnings":      warnings,
		"filter":        filter,
	})
}
