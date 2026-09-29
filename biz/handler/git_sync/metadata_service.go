package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/githubapi"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// MetadataBackup POST /api/v1/ops/metadata-backup
// 抓取 issues/PR/labels/milestones/releases 元数据快照,
// 可选下载 source archive / GitHub Release 附件 / Gists。
// 请求体绑定 biz/model/ops(IDL 生成,snake_case 标签)。
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
	maxItems := int(req.MaxItems)
	if maxItems <= 0 {
		maxItems = 500
	}
	if maxItems > 2000 {
		maxItems = 2000
	}

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
	snapDir := filepath.Join(backupDir, "metadata", textutil.SanitizePathToken(req.RepoKey),
		time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(snapDir, 0o750); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	snap := &ops.MetadataSnapshot{
		RepoKey:   req.RepoKey,
		Platform:  plat.Type,
		Owner:     repo.PlatformOwner,
		Repo:      repo.PlatformRepo,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Dir:       snapDir,
		Counts:    map[string]int32{},
		Files:     []string{},
		Archives:  []string{},
		Assets:    []string{},
		Warnings:  []string{},
	}

	collectLabelsMilestones(ctx, prov, repo, snap)
	if optBoolDefault(req.WithIssues) {
		collectIssues(ctx, prov, repo, maxItems, snap)
	}
	if optBoolDefault(req.WithPRs) {
		collectPullRequests(ctx, prov, repo, maxItems, snap)
	}
	releases := collectReleases(ctx, prov, repo, req, maxItems, snap)
	if optBoolDefault(req.WithArchives) {
		downloadSourceArchives(ctx, prov, repo, releases, snapDir, snap)
	}
	if optBoolDefault(req.WithAssets) && githubapi.IsGitHub(plat.Type) {
		downloadReleaseAssets(ctx, plat, repo, snapDir, snap)
	}
	if optBoolDefault(req.WithGists) && githubapi.IsGitHub(plat.Type) {
		backupGists(ctx, plat, snapDir, snap)
	}

	writeSnapshotJSON(snap, snapDir)
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
		response.Success(c, &ops.ListMetadataBackupsResp{Items: []*ops.MetadataSnapshot{}, Total: 0})
		return
	}
	filter := c.Query("repo_key")
	items := []*ops.MetadataSnapshot{}
	root := filepath.Join(backupDir, "metadata")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "manifest.json" {
			return nil
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // 内部路径
		if rerr != nil {
			return nil
		}
		var snap ops.MetadataSnapshot
		if json.Unmarshal(data, &snap) != nil {
			return nil
		}
		if filter != "" && snap.RepoKey != filter {
			return nil
		}
		items = append(items, &snap)
		return nil
	})
	response.Success(c, &ops.ListMetadataBackupsResp{Items: items, Total: int64(len(items))})
}

// ===== 收集器:单一职责,便于复用与测试 =====

func collectLabelsMilestones(ctx context.Context, prov sdkprov.Provider, repo *corebridge.Repo, snap *ops.MetadataSnapshot) {
	if im, ok := prov.(sdkprov.IssueManager); ok {
		labels, lerr := im.ListIssueLabels(ctx, repo.PlatformOwner, repo.PlatformRepo)
		if lerr != nil {
			snap.Warnings = append(snap.Warnings, "labels: "+lerr.Error())
		} else {
			writeSnapshotPart(snap, "labels.json", labels)
			snap.Counts["labels"] = int32(len(labels))
		}
	}
	if mm, ok := prov.(sdkprov.MilestoneManager); ok {
		ms, merr := mm.ListMilestones(ctx, repo.PlatformOwner, repo.PlatformRepo, sdkprov.ListMilestonesOptions{})
		if merr != nil {
			snap.Warnings = append(snap.Warnings, "milestones: "+merr.Error())
		} else {
			writeSnapshotPart(snap, "milestones.json", ms)
			snap.Counts["milestones"] = int32(len(ms))
		}
	}
}

func collectIssues(ctx context.Context, prov sdkprov.Provider, repo *corebridge.Repo, maxItems int, snap *ops.MetadataSnapshot) {
	im, ok := prov.(sdkprov.IssueManager)
	if !ok {
		return
	}
	issues, ierr := listIssues(ctx, prov, repo, "all", maxItems)
	if ierr != nil {
		snap.Warnings = append(snap.Warnings, "issues: "+ierr.Error())
		return
	}
	for _, iss := range issues {
		comments, cerr := im.ListIssueComments(ctx, repo.PlatformOwner, repo.PlatformRepo, iss.Number)
		if cerr == nil {
			iss.CommentList = comments
		}
	}
	writeSnapshotPart(snap, "issues.json", issues)
	snap.Counts["issues"] = int32(len(issues))
}

func collectPullRequests(ctx context.Context, prov sdkprov.Provider, repo *corebridge.Repo, maxItems int, snap *ops.MetadataSnapshot) {
	cm, ok := prov.(sdkprov.ChangeRequestManager)
	if !ok {
		return
	}
	prs, _, perr := cm.ListCRs(ctx, sdkprov.ListCROptions{
		Owner: repo.PlatformOwner, Repo: repo.PlatformRepo, PerPage: maxItems,
	})
	if perr != nil {
		snap.Warnings = append(snap.Warnings, "pull_requests: "+perr.Error())
		return
	}
	if len(prs) > maxItems {
		prs = prs[:maxItems]
	}
	writeSnapshotPart(snap, "pull_requests.json", prs)
	snap.Counts["pull_requests"] = int32(len(prs))
}

func collectReleases(ctx context.Context, prov sdkprov.Provider, repo *corebridge.Repo,
	req ops.MetadataBackupReq, maxItems int, snap *ops.MetadataSnapshot) []*sdkprov.ReleaseInfo {
	if !optBoolDefault(req.WithReleases) {
		return nil
	}
	rm, ok := prov.(sdkprov.ReleaseManager)
	if !ok {
		return nil
	}
	rels, rerr := rm.ListReleases(ctx, repo.PlatformOwner, repo.PlatformRepo)
	if rerr != nil {
		snap.Warnings = append(snap.Warnings, "releases: "+rerr.Error())
		return nil
	}
	if len(rels) > maxItems {
		rels = rels[:maxItems]
	}
	writeSnapshotPart(snap, "releases.json", rels)
	snap.Counts["releases"] = int32(len(rels))
	return rels
}

func downloadSourceArchives(ctx context.Context, prov sdkprov.Provider, repo *corebridge.Repo,
	releases []*sdkprov.ReleaseInfo, snapDir string, snap *ops.MetadataSnapshot) {
	if len(releases) == 0 {
		return
	}
	rm, ok := prov.(sdkprov.ReleaseManager)
	if !ok {
		return
	}
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
	snap.Counts["archives"] = int32(len(snap.Archives))
}

func downloadReleaseAssets(ctx context.Context, plat *corebridge.Platform, repo *corebridge.Repo, snapDir string, snap *ops.MetadataSnapshot) {
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
		return
	}
	snap.Assets = saved
	snap.Counts["release_assets"] = int32(len(saved))
}

func backupGists(ctx context.Context, plat *corebridge.Platform, snapDir string, snap *ops.MetadataSnapshot) {
	token := plat.AccessToken
	gistDir := filepath.Join(snapDir, "gists")
	gc, warns, gerr := githubapi.BackupGists(ctx, plat.APIURL, token, gistDir, 200)
	snap.Warnings = append(snap.Warnings, warns...)
	if gerr != nil {
		snap.Warnings = append(snap.Warnings, "gists: "+gerr.Error())
		return
	}
	snap.Counts["gists"] = int32(gc)
}

// writeSnapshotPart 写出分片 JSON 并登记文件名。
func writeSnapshotPart(snap *ops.MetadataSnapshot, name string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		snap.Warnings = append(snap.Warnings, name+": marshal: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(snap.Dir, name), data, 0o600); err != nil {
		snap.Warnings = append(snap.Warnings, name+": write: "+err.Error())
		return
	}
	snap.Files = append(snap.Files, name)
}

func writeSnapshotJSON(snap *ops.MetadataSnapshot, snapDir string) {
	writeSnapshotPart(snap, "manifest.json", snap)
}

