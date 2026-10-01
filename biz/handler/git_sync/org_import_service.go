package git_sync

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/githubapi"
	"github.com/yi-nology/git-ferry/internal/orgmap"
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
	policy, err := orgmap.ParsePolicy(req.OrgMapping)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := orgmap.Map(policy, &orgmap.Input{
		SourceKey:       req.SourceRepoKey,
		IsPersonal:      req.IsPersonal,
		TargetNamespace: req.TargetOrg,
		TargetPlatform:  req.TargetPlatform,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"policy":       string(policy),
		"target_owner": out.TargetOwner,
		"target_repo":  out.TargetRepo,
		"target_key":   out.TargetKey,
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

	// GitHub 走匿名列仓（gitea-mirror public mode）；其它平台回落全量导入
	items := []map[string]any{}
	warnings := []string{}
	if githubapi.IsGitHub(plat.Type) {
		repos, err := githubapi.ListPublicOrgRepos(ctx, plat.APIURL, plat.AccessToken, req.Org, max)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		for i := range repos {
			r := &repos[i]
			items = append(items, map[string]any{
				"full_name": r.FullName, "clone_url": r.CloneURL,
				"fork": r.Fork, "archived": r.Archived, "stars": r.Stars,
			})
		}
		warnings = append(warnings, "匿名 GitHub API 60 req/h；大组织可能只拉到部分元数据")
	}

	filter := &corebridge.RepoImportFilter{
		ExcludeArchived: optBool(req.ExcludeArchived),
		ExcludeForks:    optBool(req.ExcludeForks),
		MinStars:        int(req.MinStars),
	}
	imported := 0
	if !dryRun {
		n, err := svc.SyncPlatformReposFiltered(ctx, req.PlatformKey, filter)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		imported = n
	}

	createdTasks := 0
	if !dryRun && req.CreateTasks {
		repos, _, lerr := svc.ListRepos(ctx, 0, 200)
		if lerr == nil {
			for _, r := range repos {
				if !strings.Contains(r.Key, "/"+req.Org+"/") && !strings.HasPrefix(r.Key, req.PlatformKey+"/"+req.Org+"/") {
					continue
				}
				target := r.Key
				if req.TargetOrg != "" {
					if out, merr := orgmap.Map(orgmap.PolicySingle, &orgmap.Input{
						SourceKey: r.Key, TargetNamespace: req.TargetOrg, TargetPlatform: req.TargetPlatform,
					}); merr == nil {
						target = out.TargetKey
						if target == "" {
							target = out.TargetOwner + "/" + out.TargetRepo
						}
					}
				}
				name := r.Name + "-mirror"
				_, cerr := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
					Name:          name,
					SourceRepoKey: r.Key,
					SourceBranch:  "*",
					TargetRepoKey: target,
					TargetBranch:  "*",
					SyncMode:      "all",
				})
				if cerr == nil {
					createdTasks++
				}
			}
		}
	}

	if !dryRun {
		recordAudit(ctx, c, "import_public_org", "platform", req.PlatformKey,
			"导入公共组织 "+req.Org+" 仓库数="+strconv.Itoa(imported)+" 任务="+strconv.Itoa(createdTasks))
	} else {
		warnings = append(warnings, "dry_run=true：未写入")
	}
	response.Success(c, map[string]any{
		"org":           req.Org,
		"dry_run":       dryRun,
		"found":         len(items),
		"imported":      imported,
		"created_tasks": createdTasks,
		"items":         items,
		"warnings":      warnings,
		"filter":        filter,
	})
}
