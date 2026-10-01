package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// MetadataRestore POST /api/v1/ops/metadata-restore
// 将 metadata-backup 快照回灌到目标仓：labels → milestones → issues → PRs(以 issue 形态) → releases。
// dry_run 缺省 true（安全默认）；overwrite=false 时同名跳过。
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
	backupDir := svc.BackupDir()
	if backupDir == "" {
		response.BadRequest(c, "sync.backup_dir not configured")
		return
	}

	snapDir := req.GetSnapshotDir()
	if snapDir == "" {
		latest, lerr := latestMetadataSnapshot(backupDir, req.RepoKey)
		if lerr != nil {
			response.BadRequest(c, lerr.Error())
			return
		}
		snapDir = latest
	}
	snap, err := loadMetadataSnapshot(snapDir)
	if err != nil {
		response.BadRequest(c, "load snapshot: "+err.Error())
		return
	}

	repo, err := svc.GetRepo(ctx, req.RepoKey)
	if err != nil || repo == nil {
		response.NotFound(c, "repo not found")
		return
	}
	plat := resolveTargetPlatform(ctx, svc, repo, req.GetTargetPlatform())
	if plat == nil {
		response.NotFound(c, "target platform not found")
		return
	}
	owner := req.GetTargetOwner()
	if owner == "" {
		owner = repo.PlatformOwner
	}
	repoName := req.GetTargetRepo()
	if repoName == "" {
		repoName = repo.PlatformRepo
	}
	prov, err := newIssueProvider(plat, repo.AccessToken)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	kinds := normalizeRestoreKinds(req.GetKinds(), snap)
	result := &ops.MetadataRestoreResult{
		SnapshotDir: snapDir,
		Target:      fmt.Sprintf("%s/%s/%s", plat.Type, owner, repoName),
		Stats:       map[string]*ops.RestoreKindStat{},
		Warnings:    []string{},
		DryRun:      dryRun,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	for _, k := range kinds {
		result.Stats[k] = &ops.RestoreKindStat{}
	}

	target := restoreTarget{owner: owner, repo: repoName, plat: plat, prov: prov, overwrite: overwrite, dryRun: dryRun}
	for _, kind := range kinds {
		stat := result.Stats[kind]
		switch kind {
		case "labels":
			restoreLabels(ctx, target, snapDir, stat, result)
		case "milestones":
			restoreMilestones(ctx, target, snapDir, stat, result)
		case "issues":
			restoreIssues(ctx, target, snapDir, stat, result, false)
		case "prs":
			restoreIssues(ctx, target, snapDir, stat, result, true)
		case "releases":
			restoreReleases(ctx, target, snapDir, stat, result)
		default:
			result.Warnings = append(result.Warnings, "unknown kind: "+kind)
		}
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	writeRestoreHistory(backupDir, req.RepoKey, result)
	if dryRun {
		result.Warnings = append(result.Warnings, "dry_run=true：未写入目标；去掉 dry_run 执行")
	} else {
		recordAudit(ctx, c, "metadata_restore", "backup", req.RepoKey,
			fmt.Sprintf("元数据回灌 target=%s dry_run=false", result.Target))
	}
	response.Success(c, result)
}

type restoreTarget struct {
	owner     string
	repo      string
	plat      *corebridge.Platform
	prov      sdkprov.Provider
	overwrite bool
	dryRun    bool
}

func resolveTargetPlatform(ctx context.Context, svc *corebridge.Service, repo *corebridge.Repo, key string) *corebridge.Platform {
	if key == "" {
		plat, err := svc.GetPlatformByID(ctx, repo.PlatformID)
		if err != nil {
			return nil
		}
		return plat
	}
	plat, err := svc.GetPlatform(ctx, key)
	if err != nil {
		return nil
	}
	return plat
}

func latestMetadataSnapshot(backupDir, repoKey string) (string, error) {
	root := filepath.Join(backupDir, "metadata", textutil.SanitizePathToken(repoKey))
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("no snapshot for %s: %w", repoKey, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no snapshot for %s", repoKey)
	}
	sort.Strings(dirs)
	return filepath.Join(root, dirs[len(dirs)-1]), nil
}

func loadMetadataSnapshot(dir string) (*ops.MetadataSnapshot, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json")) //nolint:gosec // 内部备份路径
	if err != nil {
		return nil, err
	}
	var snap ops.MetadataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func normalizeRestoreKinds(kinds []string, snap *ops.MetadataSnapshot) []string {
	available := map[string]bool{}
	for _, f := range snap.Files {
		switch f {
		case "labels.json":
			available["labels"] = true
		case "milestones.json":
			available["milestones"] = true
		case "issues.json":
			available["issues"] = true
		case "pull_requests.json":
			available["prs"] = true
		case "releases.json":
			available["releases"] = true
		}
	}
	if len(kinds) == 0 {
		out := []string{"labels", "milestones", "issues", "prs", "releases"}
		filtered := out[:0]
		for _, k := range out {
			if available[k] {
				filtered = append(filtered, k)
			}
		}
		return filtered
	}
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		k = strings.TrimSpace(strings.ToLower(k))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

func readSnapshotJSON(dir, name string, v any) error {
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // 内部备份路径
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func restoreLabels(ctx context.Context, t restoreTarget, dir string, stat *ops.RestoreKindStat, result *ops.MetadataRestoreResult) {
	var labels []*sdkprov.IssueLabel
	if err := readSnapshotJSON(dir, "labels.json", &labels); err != nil {
		result.Warnings = append(result.Warnings, "labels.json: "+err.Error())
		return
	}
	lm, ok := t.prov.(sdkprov.LabelManager)
	if !ok {
		result.Warnings = append(result.Warnings, "target does not support LabelManager")
		return
	}
	existing := map[string]bool{}
	if cur, err := lm.ListLabels(ctx, t.owner, t.repo, sdkprov.ListLabelsOptions{PerPage: 100}); err == nil {
		for _, l := range cur {
			existing[l.Name] = true
		}
	}
	for _, lb := range labels {
		stat.Planned++
		if lb == nil || lb.Name == "" {
			stat.Skipped++
			continue
		}
		if existing[lb.Name] {
			if !t.overwrite {
				stat.Skipped++
				continue
			}
			if t.dryRun {
				stat.Created++
				continue
			}
			if _, err := lm.UpdateLabel(ctx, t.owner, t.repo, lb.Name, sdkprov.UpdateLabelOptions{
				Color: &lb.Color,
			}); err != nil {
				stat.Failed++
				result.Warnings = append(result.Warnings, "update label "+lb.Name+": "+err.Error())
				continue
			}
			stat.Created++
			continue
		}
		if t.dryRun {
			stat.Created++
			continue
		}
		if _, err := lm.CreateLabel(ctx, t.owner, t.repo, sdkprov.CreateLabelOptions{
			Name:  lb.Name,
			Color: lb.Color,
		}); err != nil {
			stat.Failed++
			result.Warnings = append(result.Warnings, "create label "+lb.Name+": "+err.Error())
			continue
		}
		stat.Created++
	}
}

func restoreMilestones(ctx context.Context, t restoreTarget, dir string, stat *ops.RestoreKindStat, result *ops.MetadataRestoreResult) {
	var ms []*sdkprov.Milestone
	if err := readSnapshotJSON(dir, "milestones.json", &ms); err != nil {
		result.Warnings = append(result.Warnings, "milestones.json: "+err.Error())
		return
	}
	mm, ok := t.prov.(sdkprov.MilestoneManager)
	if !ok {
		result.Warnings = append(result.Warnings, "target does not support MilestoneManager")
		return
	}
	existing := map[string]bool{}
	if cur, err := mm.ListMilestones(ctx, t.owner, t.repo, sdkprov.ListMilestonesOptions{PerPage: 100}); err == nil {
		for _, m := range cur {
			existing[m.Title] = true
		}
	}
	for _, m := range ms {
		stat.Planned++
		if m == nil || m.Title == "" {
			stat.Skipped++
			continue
		}
		if existing[m.Title] {
			stat.Skipped++
			continue
		}
		if t.dryRun {
			stat.Created++
			continue
		}
		opts := sdkprov.CreateMilestoneOptions{Title: m.Title, Description: m.Description, DueOn: m.DueOn}
		if _, err := mm.CreateMilestone(ctx, t.owner, t.repo, opts); err != nil {
			stat.Failed++
			result.Warnings = append(result.Warnings, "create milestone "+m.Title+": "+err.Error())
			continue
		}
		stat.Created++
	}
}

// restoreIssues 回灌 issues；asPR=true 时读 pull_requests.json 并以 issue 形态落盘（标注来源）。
func restoreIssues(ctx context.Context, t restoreTarget, dir string, stat *ops.RestoreKindStat, result *ops.MetadataRestoreResult, asPR bool) {
	im, ok := t.prov.(sdkprov.IssueManager)
	if !ok {
		result.Warnings = append(result.Warnings, "target does not support IssueManager")
		return
	}
	existingTitles := map[string]bool{}
	if cur, _, err := im.ListIssues(ctx, sdkprov.ListIssuesOptions{
		Owner: t.owner, Repo: t.repo, State: sdkprov.IssueState("all"), PerPage: 100,
	}); err == nil {
		for _, iss := range cur {
			existingTitles[iss.Title] = true
		}
	}

	if asPR {
		var prs []*sdkprov.ChangeRequest
		if err := readSnapshotJSON(dir, "pull_requests.json", &prs); err != nil {
			result.Warnings = append(result.Warnings, "pull_requests.json: "+err.Error())
			return
		}
		for _, pr := range prs {
			stat.Planned++
			if pr == nil || pr.Title == "" {
				stat.Skipped++
				continue
			}
			if existingTitles[pr.Title] {
				stat.Skipped++
				continue
			}
			body := prBody(pr)
			if t.dryRun {
				stat.Created++
				continue
			}
			created, err := im.CreateIssue(ctx, sdkprov.CreateIssueOptions{
				Owner: t.owner, Repo: t.repo, Title: pr.Title, Body: body,
			})
			if err != nil {
				stat.Failed++
				result.Warnings = append(result.Warnings, "create pr-as-issue "+pr.Title+": "+err.Error())
				continue
			}
			stat.Created++
			existingTitles[pr.Title] = true
			_ = created
		}
		return
	}

	var issues []*issueRow
	if err := readSnapshotJSON(dir, "issues.json", &issues); err != nil {
		result.Warnings = append(result.Warnings, "issues.json: "+err.Error())
		return
	}
	for _, iss := range issues {
		stat.Planned++
		if iss == nil || iss.Title == "" {
			stat.Skipped++
			continue
		}
		if existingTitles[iss.Title] {
			stat.Skipped++
			continue
		}
		body := issueBody(iss)
		if t.dryRun {
			stat.Created++
			continue
		}
		created, err := im.CreateIssue(ctx, sdkprov.CreateIssueOptions{
			Owner: t.owner, Repo: t.repo, Title: iss.Title, Body: body,
			Labels: iss.Labels,
		})
		if err != nil {
			stat.Failed++
			result.Warnings = append(result.Warnings, "create issue "+iss.Title+": "+err.Error())
			continue
		}
		stat.Created++
		existingTitles[iss.Title] = true
		for _, cm := range iss.CommentList {
			if cm == nil || cm.Body == "" {
				continue
			}
			if _, cerr := im.CreateIssueComment(ctx, t.owner, t.repo, created.Number, cm.Body); cerr != nil {
				result.Warnings = append(result.Warnings, "comment on "+iss.Title+": "+cerr.Error())
			}
		}
		if strings.EqualFold(iss.State, "closed") {
			if _, cerr := im.CloseIssue(ctx, t.owner, t.repo, created.Number); cerr != nil {
				result.Warnings = append(result.Warnings, "close issue "+iss.Title+": "+cerr.Error())
			}
		}
	}
}

func issueBody(iss *issueRow) string {
	var b strings.Builder
	b.WriteString("<!-- gitferry-restore:issue -->\n")
	if iss.WebURL != "" {
		b.WriteString("> 源：" + iss.WebURL + "\n\n")
	}
	if iss.Author != "" {
		b.WriteString("原作者：@" + iss.Author + "  ·  原编号：" + iss.Number + "\n\n")
	}
	b.WriteString(iss.Body)
	return b.String()
}

func prBody(pr *sdkprov.ChangeRequest) string {
	var b strings.Builder
	b.WriteString("<!-- gitferry-restore:pr -->\n")
	b.WriteString("> 以 issue 形态从 PR 快照回灌（不创建真实 PR）\n\n")
	_, _ = fmt.Fprintf(&b, "原编号：#%s  ·  `%s` → `%s`\n\n", pr.Number, pr.SourceBranch, pr.TargetBranch)
	b.WriteString(pr.Description)
	return b.String()
}

func restoreReleases(ctx context.Context, t restoreTarget, dir string, stat *ops.RestoreKindStat, result *ops.MetadataRestoreResult) {
	var rels []*sdkprov.ReleaseInfo
	if err := readSnapshotJSON(dir, "releases.json", &rels); err != nil {
		result.Warnings = append(result.Warnings, "releases.json: "+err.Error())
		return
	}
	rm, ok := t.prov.(sdkprov.ReleaseManager)
	if !ok {
		result.Warnings = append(result.Warnings, "target does not support ReleaseManager")
		return
	}
	existing := map[string]bool{}
	if cur, err := rm.ListReleases(ctx, t.owner, t.repo); err == nil {
		for _, r := range cur {
			existing[r.TagName] = true
		}
	}
	for _, rel := range rels {
		stat.Planned++
		if rel == nil || rel.TagName == "" {
			stat.Skipped++
			continue
		}
		if existing[rel.TagName] {
			stat.Skipped++
			continue
		}
		if t.dryRun {
			stat.Created++
			continue
		}
		_, err := rm.CreateRelease(ctx, t.owner, t.repo, sdkprov.CreateReleaseOptions{
			TagName:    rel.TagName,
			Title:      rel.Title,
			Body:       rel.Body,
			Draft:      rel.Draft,
			Prerelease: rel.Prerelease,
		})
		if err != nil {
			stat.Failed++
			result.Warnings = append(result.Warnings, "create release "+rel.TagName+": "+err.Error())
			continue
		}
		stat.Created++
	}
}

func writeRestoreHistory(backupDir, repoKey string, result *ops.MetadataRestoreResult) {
	dir := filepath.Join(backupDir, "metadata-restore", textutil.SanitizePathToken(repoKey))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, time.Now().UTC().Format("20060102-150405")+".json"), data, 0o600)
}
