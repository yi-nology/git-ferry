package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/githubapi"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// MetadataBackupReq 元数据资产快照请求。
type MetadataBackupReq struct {
	RepoKey string `json:"repo_key" form:"repo_key" query:"repo_key"`
	// WithIssues / WithPRs / WithReleases / WithArchives 默认全开
	WithIssues   *bool `json:"with_issues" form:"with_issues"`
	WithPRs      *bool `json:"with_prs" form:"with_prs"`
	WithReleases *bool `json:"with_releases" form:"with_releases"`
	WithArchives *bool `json:"with_archives" form:"with_archives"`
	// WithAssets 下载 GitHub Release 二进制附件(仅 GitHub)
	WithAssets *bool `json:"with_assets" form:"with_assets"`
	// WithGists 附带备份当前 token 可见的 gists(仅 GitHub)
	WithGists *bool `json:"with_gists" form:"with_gists"`
	MaxItems  int   `json:"max_items" form:"max_items"`
}

// metadataSnapshot 单次元数据快照的清单。
type metadataSnapshot struct {
	RepoKey   string         `json:"repo_key"`
	Platform  string         `json:"platform"`
	Owner     string         `json:"owner"`
	Repo      string         `json:"repo"`
	CreatedAt time.Time      `json:"created_at"`
	Dir       string         `json:"dir"`
	Counts    map[string]int `json:"counts"`
	Files     []string       `json:"files"`
	Archives  []string       `json:"archives,omitempty"`
	Assets    []string       `json:"assets,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// MetadataBackup POST /api/v1/ops/metadata-backup
// 抓取 issues/PR/labels/milestones/releases 元数据快照,
// 可选下载各 release 的 source archive(git bundle 不含 release 二进制,归档可补源码快照)。
func MetadataBackup(ctx context.Context, c *app.RequestContext) {
	var req MetadataBackupReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}
	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = 500
	}
	if maxItems > 2000 {
		maxItems = 2000
	}
	with := func(p *bool) bool { return p == nil || *p }

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	repo, err := svc.GetRepo(ctx, req.RepoKey)
	if err != nil || repo == nil {
		response.NotFound(c, "repo not found")
		return
	}
	plat, err := svc.GetPlatformByID(ctx, repo.PlatformID)
	if err != nil || plat == nil {
		response.NotFound(c, "platform not found")
		return
	}

	prov, err := newIssueProvider(plat, repo.AccessToken)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	backupDir := svc.BackupDir()
	if backupDir == "" {
		response.BadRequest(c, "sync.backup_dir not configured")
		return
	}
	snapDir := filepath.Join(backupDir, "metadata", textutil.SanitizePathToken(req.RepoKey), time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(snapDir, 0o750); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	snap := &metadataSnapshot{
		RepoKey:   req.RepoKey,
		Platform:  plat.Type,
		Owner:     repo.PlatformOwner,
		Repo:      repo.PlatformRepo,
		CreatedAt: time.Now().UTC(),
		Dir:       snapDir,
		Counts:    map[string]int{},
	}

	// labels
	if im, ok := prov.(sdkprov.IssueManager); ok {
		labels, lerr := im.ListIssueLabels(ctx, repo.PlatformOwner, repo.PlatformRepo)
		if lerr != nil {
			snap.Warnings = append(snap.Warnings, "labels: "+lerr.Error())
		} else {
			writeJSON(snap, snapDir, "labels.json", labels)
			snap.Counts["labels"] = len(labels)
		}
	}

	// milestones
	if mm, ok := prov.(sdkprov.MilestoneManager); ok {
		ms, merr := mm.ListMilestones(ctx, repo.PlatformOwner, repo.PlatformRepo, sdkprov.ListMilestonesOptions{})
		if merr != nil {
			snap.Warnings = append(snap.Warnings, "milestones: "+merr.Error())
		} else {
			writeJSON(snap, snapDir, "milestones.json", ms)
			snap.Counts["milestones"] = len(ms)
		}
	}

	// issues + comments
	if with(req.WithIssues) {
		if im, ok := prov.(sdkprov.IssueManager); ok {
			issues, ierr := listIssues(ctx, prov, repo, "all", maxItems)
			if ierr != nil {
				snap.Warnings = append(snap.Warnings, "issues: "+ierr.Error())
			} else {
				for _, iss := range issues {
					comments, cerr := im.ListIssueComments(ctx, repo.PlatformOwner, repo.PlatformRepo, iss.Number)
					if cerr == nil {
						iss.CommentList = comments
					}
				}
				writeJSON(snap, snapDir, "issues.json", issues)
				snap.Counts["issues"] = len(issues)
			}
		}
	}

	// pull requests
	if with(req.WithPRs) {
		if cm, ok := prov.(sdkprov.ChangeRequestManager); ok {
			prs, _, perr := cm.ListCRs(ctx, sdkprov.ListCROptions{
				Owner:   repo.PlatformOwner,
				Repo:    repo.PlatformRepo,
				PerPage: maxItems,
			})
			if perr != nil {
				snap.Warnings = append(snap.Warnings, "pull_requests: "+perr.Error())
			} else {
				if len(prs) > maxItems {
					prs = prs[:maxItems]
				}
				writeJSON(snap, snapDir, "pull_requests.json", prs)
				snap.Counts["pull_requests"] = len(prs)
			}
		}
	}

	// releases + optional source archives
	var releases []*sdkprov.ReleaseInfo
	if with(req.WithReleases) {
		if rm, ok := prov.(sdkprov.ReleaseManager); ok {
			rels, rerr := rm.ListReleases(ctx, repo.PlatformOwner, repo.PlatformRepo)
			if rerr != nil {
				snap.Warnings = append(snap.Warnings, "releases: "+rerr.Error())
			} else {
				if len(rels) > maxItems {
					rels = rels[:maxItems]
				}
				releases = rels
				writeJSON(snap, snapDir, "releases.json", rels)
				snap.Counts["releases"] = len(rels)
			}
		}
	}

	// source archives(每个 release tag 一份 tar.gz)
	if with(req.WithArchives) && len(releases) > 0 {
		if rm, ok := prov.(sdkprov.ReleaseManager); ok {
			archDir := filepath.Join(snapDir, "archives")
			_ = os.MkdirAll(archDir, 0o750)
			for _, rel := range releases {
				if rel.TagName == "" {
					continue
				}
				data, aerr := rm.GetArchive(ctx, repo.PlatformOwner, repo.PlatformRepo, rel.TagName, "tar.gz")
				if aerr != nil {
					snap.Warnings = append(snap.Warnings, "archive "+rel.TagName+": "+aerr.Error())
					continue
				}
				name := textutil.SanitizePathToken(rel.TagName) + ".tar.gz"
				if err := os.WriteFile(filepath.Join(archDir, name), data, 0o600); err != nil {
					snap.Warnings = append(snap.Warnings, "write archive "+name+": "+err.Error())
					continue
				}
				snap.Archives = append(snap.Archives, name)
			}
			snap.Counts["archives"] = len(snap.Archives)
		}
	}

	// Release 二进制附件(git bundle 盲区,仅 GitHub)
	if with(req.WithAssets) && githubapi.IsGitHub(plat.Type) {
		token := repo.AccessToken
		if token == "" {
			token = plat.AccessToken
		}
		assetDir := filepath.Join(snapDir, "release-assets")
		saved, warns, aerr := githubapi.DownloadReleaseAssets(ctx, plat.APIURL, token,
			repo.PlatformOwner, repo.PlatformRepo, assetDir, 100)
		snap.Warnings = append(snap.Warnings, warns...)
		if aerr != nil {
			snap.Warnings = append(snap.Warnings, "release-assets: "+aerr.Error())
		} else {
			snap.Assets = saved
			snap.Counts["release_assets"] = len(saved)
		}
	}

	// Gists 附带备份(仅 GitHub)
	if with(req.WithGists) && githubapi.IsGitHub(plat.Type) {
		token := repo.AccessToken
		if token == "" {
			token = plat.AccessToken
		}
		gistDir := filepath.Join(snapDir, "gists")
		gc, warns, gerr := githubapi.BackupGists(ctx, plat.APIURL, token, gistDir, 200)
		snap.Warnings = append(snap.Warnings, warns...)
		if gerr != nil {
			snap.Warnings = append(snap.Warnings, "gists: "+gerr.Error())
		} else {
			snap.Counts["gists"] = gc
		}
	}

	writeJSON(snap, snapDir, "manifest.json", snap)
	recordAudit(ctx, c, "metadata_backup", "backup", req.RepoKey,
		fmt.Sprintf("元数据快照 issues=%d prs=%d releases=%d archives=%d assets=%d gists=%d",
			snap.Counts["issues"], snap.Counts["pull_requests"], snap.Counts["releases"],
			snap.Counts["archives"], snap.Counts["release_assets"], snap.Counts["gists"]))
	response.Success(c, snap)
}

// ListMetadataBackups GET /api/v1/ops/metadata-backups?repo_key=
func ListMetadataBackups(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	backupDir := svc.BackupDir()
	if backupDir == "" {
		response.Success(c, map[string]any{"items": []any{}, "total": 0})
		return
	}
	root := filepath.Join(backupDir, "metadata")
	items := []any{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "manifest.json" {
			return nil
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // 内部路径
		if rerr != nil {
			return nil
		}
		var snap metadataSnapshot
		if json.Unmarshal(data, &snap) != nil {
			return nil
		}
		if key := c.Query("repo_key"); key != "" && snap.RepoKey != key {
			return nil
		}
		items = append(items, snap)
		return nil
	})
	response.Success(c, map[string]any{"items": items, "total": len(items)})
}

// writeJSON 把任意对象写入快照目录,并登记到 snap.Files。
func writeJSON(snap *metadataSnapshot, dir, name string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		snap.Warnings = append(snap.Warnings, name+": marshal: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		snap.Warnings = append(snap.Warnings, name+": write: "+err.Error())
		return
	}
	snap.Files = append(snap.Files, name)
}


