package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	errors "github.com/cockroachdb/errors"
	"github.com/yi-nology/git-platform-sdk/gitbackend"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// RunManager handles sync run lifecycle operations.
type RunManager interface {
	CreateRun(task *model.SyncTask, trigger string, webhookEventID *uint) (*model.SyncRun, error)
	CreateRunStep(step *model.SyncRunStep) error
	UpdateRunStep(step *model.SyncRunStep) error
	CompleteRun(run *model.SyncRun) error
	UpdateTaskLastRun(task *model.SyncTask, run *model.SyncRun) error
	CompleteRunWithTaskUpdate(run *model.SyncRun, task *model.SyncTask) error
}

// RepoProvider provides repository lookup by key.
type RepoProvider interface {
	GetRepoByKey(key string) (*model.Repo, error)
}

// PlatformProvider provides platform lookup by ID.
type PlatformProvider interface {
	GetPlatformByID(ctx context.Context, id uint) (*model.Platform, error)
}

// Service is the interface the executor depends on.
// It exposes only the operations the executor needs — no DAO leakage.
type Service interface {
	GetTempDir(taskKey string) string
	GetConfig() *model.Config
	RunManager
	RepoProvider
	PlatformProvider
}

type Executor struct {
	service Service
	backend gitbackend.GitBackend
}

func NewExecutor(svc Service) (*Executor, error) {
	// git.backend 配置接线:空值由 SDK 自动选择(native 优先,回退 gogit)
	backendType := ""
	if cfg := svc.GetConfig(); cfg != nil {
		backendType = cfg.Git.Backend
	}
	backend, err := gitbackend.NewGitBackend(gitbackend.Options{Type: backendType})
	if err != nil {
		return nil, errors.Wrap(err, "init git backend failed")
	}
	return &Executor{
		service: svc,
		backend: backend,
	}, nil
}

func (e *Executor) Execute(ctx context.Context, task *model.SyncTask, trigger string, webhookEventID *uint) (*model.SyncRun, error) {
	run, err := e.service.CreateRun(task, trigger, webhookEventID)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()

	// Build a summary details string for backward compatibility
	var details strings.Builder
	const maxDetailsSize = 64 * 1024 // 64KB 上限,防止超长任务撑爆 DB
	fmt.Fprintf(&details, "=== Sync Task: %s ===\n", task.Name)
	fmt.Fprintf(&details, "Trigger: %s\n", trigger)
	fmt.Fprintf(&details, "Time: %s\n\n", startTime.Format(time.RFC3339))

	defer func() {
		run.EndTime = timePtr(time.Now())
		run.DurationMs = run.EndTime.Sub(startTime).Milliseconds()
		detailStr := details.String()
		if len(detailStr) > maxDetailsSize {
			detailStr = detailStr[:maxDetailsSize] + "\n... (truncated)"
		}
		run.Details = detailStr
		// 事务化:run 完成 + task 状态更新在同一事务中,避免中间崩溃导致不一致
		if err := e.service.CompleteRunWithTaskUpdate(run, task); err != nil {
			slog.Error("failed to complete sync run and update task", "error", err)
		}
	}()

	sourceRepo, err := e.service.GetRepoByKey(task.SourceRepoKey)
	if err != nil {
		return failRun(run, errors.Wrap(err, "query source repo failed"))
	}
	if sourceRepo == nil {
		return failRun(run, errors.Newf("source repo not found: %s", task.SourceRepoKey))
	}

	targetRepo, err := e.service.GetRepoByKey(task.TargetRepoKey)
	if err != nil {
		return failRun(run, errors.Wrap(err, "query target repo failed"))
	}
	if targetRepo == nil {
		return failRun(run, errors.Newf("target repo not found: %s", task.TargetRepoKey))
	}

	// 空分支回退到仓库默认分支,再回退 "main":否则 clone/fetch/push 会拼出
	// "refs/heads/:refs/heads/" 这类坏 refspec,表现为静默零推送或直接失败。
	// 注意:只用本地副本做 git 操作,不回写原 task(CompleteRun 会 Save 整行)。
	runTask := *task
	if runTask.SourceBranch == "" {
		runTask.SourceBranch = defaultBranchOf(sourceRepo)
	}
	if runTask.TargetBranch == "" {
		runTask.TargetBranch = defaultBranchOf(targetRepo)
	}

	// 预取 source/target 的 platform 记录,避免后续每次 git 操作都查 DB。
	platforms := e.prefetchPlatforms(ctx, sourceRepo, targetRepo)

	workDir := e.service.GetTempDir(task.Key)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return failRun(run, errors.Wrap(err, "create work dir failed"))
	}

	defer func() {
		// 成功:保留 workdir,下次走增量 fetch(避免每次全量 clone);
		// 非成功(失败/panic):清理,避免损坏的 .git / index.lock 影响下次执行。
		// 注:同 taskKey 的并发执行已由 Service.runningTasks 互斥,不存在并发写同一 workdir。
		if run.Status != model.StatusSuccess {
			if rmErr := os.RemoveAll(workDir); rmErr != nil {
				slog.Error("failed to cleanup temp dir", "error", rmErr, "dir", workDir)
			}
		}
	}()

	repoDir := filepath.Join(workDir, RepoDir)

	timeout := e.service.GetConfig().Sync.DefaultTimeout
	if timeout <= 0 {
		timeout = 300
	}
	execCtx, execCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer execCancel()

	// Step 1: Clone or Fetch
	var step1Name string
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		step1Name = model.StepClone
	} else {
		step1Name = model.StepFetch
	}

	step1 := e.beginStep(run.ID, step1Name)
	if step1Name == model.StepClone {
		details.WriteString("Step 1: Initial clone of source repo...\n")
		if err := e.cloneRepo(execCtx, repoDir, sourceRepo, &runTask, platforms[sourceRepo.PlatformID]); err != nil {
			e.failStep(step1, err)
			fmt.Fprintf(&details, "clone error: %v\n", err)
			return failRun(run, errors.Wrap(err, "clone failed"))
		}
	} else {
		details.WriteString("Step 1: Fetch updates from source repo...\n")
		if err := e.fetchRepo(execCtx, repoDir, &runTask, sourceRepo, platforms[sourceRepo.PlatformID]); err != nil {
			e.failStep(step1, err)
			fmt.Fprintf(&details, "fetch error: %v\n", err)
			return failRun(run, errors.Wrap(err, "fetch failed"))
		}
	}
	e.completeStep(step1, "")
	details.WriteString("Step 1: completed\n")

	// Step 2: Ensure target remote
	step2 := e.beginStep(run.ID, model.StepEnsureRemote)
	details.WriteString("\nStep 2: Ensure target remote exists...\n")
	if err := e.ensureRemote(execCtx, repoDir, targetRepo); err != nil {
		e.failStep(step2, err)
		fmt.Fprintf(&details, "add remote error: %v\n", err)
		return failRun(run, errors.Wrap(err, "add remote failed"))
	}
	e.completeStep(step2, "")
	details.WriteString("Step 2: completed\n")

	// Step 3: Push with retry
	step3 := e.beginStep(run.ID, model.StepPush)
	details.WriteString("\nStep 3: Push to target...\n")
	maxRetries := e.service.GetConfig().Sync.RetryCount
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var pushErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			step3.RetryCount = attempt - 1
			fmt.Fprintf(&details, "\nRetry attempt %d/%d...\n", attempt, maxRetries)
			// 退避期间监听 context,超时/取消时立即中止,不再干等
			backoff := time.Duration(attempt*model.RetryBackoffMs) * time.Millisecond
			timer := time.NewTimer(backoff)
			select {
			case <-execCtx.Done():
				timer.Stop()
				pushErr = execCtx.Err()
				fmt.Fprintf(&details, "retry aborted (context done): %v\n", pushErr)
			case <-timer.C:
			}
			if pushErr != nil {
				break
			}
		}

		pushErr = e.push(execCtx, repoDir, &runTask, targetRepo, platforms[targetRepo.PlatformID])
		if pushErr == nil {
			break
		}

		if attempt < maxRetries {
			details.WriteString("Push failed, retrying fetch...\n")
			if err := e.fetchRepo(execCtx, repoDir, &runTask, sourceRepo, platforms[sourceRepo.PlatformID]); err != nil {
				fmt.Fprintf(&details, "Retry fetch failed: %v\n", err)
			}
		}
	}

	run.RetryTotal = max(0, step3.RetryCount)

	if pushErr != nil {
		e.failStep(step3, pushErr)
		fmt.Fprintf(&details, "push error: %v\n", pushErr)
		return failRun(run, errors.Wrapf(pushErr, "push failed after %d attempts", maxRetries))
	}
	e.completeStep(step3, "")
	details.WriteString("Step 3: completed\n")

	run.Status = model.StatusSuccess
	details.WriteString("\n=== Sync completed successfully ===")
	return run, nil
}

// failRun 标记 run 失败并按错误分类填充字段,统一 Execute 的失败出口。
func failRun(run *model.SyncRun, err error) (*model.SyncRun, error) {
	run.Status = model.StatusFailed
	run.ErrorMessage = err.Error()
	run.ErrorType = ClassifyError(err)
	return run, err
}

// prefetchPlatforms 预取 source/target 的 platform 记录,返回 platformID→Platform 映射。
// 一次 Execute 内复用,避免后续每次 git 操作都查 DB。
func (e *Executor) prefetchPlatforms(ctx context.Context, repos ...*model.Repo) map[uint]*model.Platform {
	platforms := make(map[uint]*model.Platform, 2)
	for _, repo := range repos {
		if repo.PlatformID == 0 {
			continue
		}
		if _, ok := platforms[repo.PlatformID]; ok {
			continue
		}
		p, err := e.service.GetPlatformByID(ctx, repo.PlatformID)
		if err != nil {
			slog.Warn("prefetch platform failed", "platformID", repo.PlatformID, "error", err)
			continue
		}
		platforms[repo.PlatformID] = p
	}
	return platforms
}

// authConfig 构建 git 认证配置。platform 可选(为 nil 时回退查 DB)。
func (e *Executor) authConfig(ctx context.Context, repo *model.Repo, platform *model.Platform) gitbackend.AuthConfig {
	var skipTLS bool
	var platformToken string
	if platform != nil {
		skipTLS = platform.SkipTLSVerify
		platformToken = platform.AccessToken
	} else if repo.PlatformID > 0 {
		// 回退:platform 未预取时仍查一次(兼容直接调用)
		if p, err := e.service.GetPlatformByID(ctx, repo.PlatformID); err == nil && p != nil {
			skipTLS = p.SkipTLSVerify
			platformToken = p.AccessToken
		}
	}
	token := repo.AccessToken
	if token == "" {
		token = platformToken
	}
	if token != "" {
		slog.Debug("using access token", "repo", repo.Key, "fromPlatform", repo.AccessToken == "")
		// git over HTTPS 走 HTTP Basic(占位用户名 + token 作密码)。
		// 不能用 Bearer:xhttp.TokenAuth 发 Authorization: Bearer 头,
		// GitLab/GitCode 等平台的 git 端点只认 Basic,Bearer 会报
		// "HTTP Basic: Access denied"。
		auth := gitbackend.NewTokenAuth(token)
		auth.InsecureSkipTLS = skipTLS
		return auth
	}
	slog.Debug("no auth configured", "repo", repo.Key)
	return gitbackend.AuthConfig{Type: gitbackend.AuthNone, InsecureSkipTLS: skipTLS}
}

// beginStep creates a new step record in "running" state.
func (e *Executor) beginStep(runID uint, stepName string) *model.SyncRunStep {
	step := &model.SyncRunStep{
		RunID:     runID,
		StepName:  stepName,
		Status:    model.StatusRunning,
		StartTime: time.Now(),
	}
	if err := e.service.CreateRunStep(step); err != nil {
		slog.Error("failed to create run step", "step", stepName, "error", err)
	}
	return step
}

// completeStep marks a step as successfully completed.
func (e *Executor) completeStep(step *model.SyncRunStep, output string) {
	now := time.Now()
	step.EndTime = &now
	step.DurationMs = now.Sub(step.StartTime).Milliseconds()
	step.Status = model.StatusSuccess
	step.Output = output
	if err := e.service.UpdateRunStep(step); err != nil {
		slog.Error("failed to update run step", "step", step.StepName, "error", err)
	}
}

// failStep marks a step as failed with error classification.
func (e *Executor) failStep(step *model.SyncRunStep, err error) {
	now := time.Now()
	step.EndTime = &now
	step.DurationMs = now.Sub(step.StartTime).Milliseconds()
	step.Status = model.StatusFailed
	step.ErrorMsg = err.Error()
	step.ErrorType = ClassifyError(err)
	if updateErr := e.service.UpdateRunStep(step); updateErr != nil {
		slog.Error("failed to update run step", "step", step.StepName, "error", updateErr)
	}
}

func (e *Executor) cloneRepo(ctx context.Context, dir string, repo *model.Repo, task *model.SyncTask, platform *model.Platform) error {
	// 全量克隆:同步要把源仓库推到目标,浅克隆(depth=1)缺完整历史,
	// 首次推送到空目标会被平台拒绝(shallow update not allowed),
	// 且 gogit 会把该拒绝误报为 already up-to-date。
	// workdir 成功后保留,后续执行走增量 fetch,全量克隆只是一次性成本。
	return e.backend.Clone(ctx, gitbackend.CloneOptions{
		URL:          repo.CloneURL,
		Path:         dir,
		Branch:       task.SourceBranch,
		SingleBranch: true,
		Auth:         e.authConfig(ctx, repo, platform),
	})
}

func (e *Executor) fetchRepo(ctx context.Context, dir string, task *model.SyncTask, repo *model.Repo, platform *model.Platform) error {
	// 克隆地址在 DB 里被修正(私有实例重写/仓库迁移)后,既有 workdir 的
	// origin 仍指向旧地址;不同步会一直从错误源 fetch。
	if err := e.syncRemoteURL(ctx, dir, RemoteOrigin, repo.CloneURL); err != nil {
		slog.Warn("fetchRepo: update origin URL failed", "error", err, "dir", dir)
	}

	_, err := e.backend.Fetch(ctx, gitbackend.FetchOptions{
		RepoPath: dir,
		Remote:   RemoteOrigin,
		Branches: []string{task.SourceBranch},
		Tags:     task.GitTags,
		Prune:    task.GitPrune,
		Auth:     e.authConfig(ctx, repo, platform),
	})
	if err != nil {
		return err
	}

	// 增量同步时分支已在正确位置,仅当分支不同时才 checkout
	cur, curErr := e.backend.GetCurrentBranch(ctx, dir)
	if curErr != nil {
		slog.Warn("fetchRepo: GetCurrentBranch failed, will attempt checkout", "error", curErr, "dir", dir)
	}
	if cur != task.SourceBranch {
		return e.backend.Checkout(ctx, dir, task.SourceBranch)
	}
	return nil
}

func (e *Executor) ensureRemote(ctx context.Context, dir string, repo *model.Repo) error {
	// 先看 remote 是否已配置;存在时再核对 URL(目标 clone_url 被重写后
	// 只查存在性会继续推旧地址)。native 后端 GetRemoteURL 对缺失 remote
	// 不返回 ErrRemoteNotFound,所以必须先走 GetRemotes。
	remotes, err := e.backend.GetRemotes(ctx, dir)
	if err != nil {
		return err
	}
	exists := false
	for _, name := range remotes {
		if name == RemoteTarget {
			exists = true
			break
		}
	}
	if !exists {
		return e.backend.AddRemote(ctx, dir, RemoteTarget, repo.CloneURL)
	}
	return e.syncRemoteURL(ctx, dir, RemoteTarget, repo.CloneURL)
}

// syncRemoteURL 确保 named remote 指向 wantURL(已存在时按需重建)。
func (e *Executor) syncRemoteURL(ctx context.Context, dir, name, wantURL string) error {
	if wantURL == "" {
		return nil
	}
	cur, err := e.backend.GetRemoteURL(ctx, dir, name)
	if err != nil {
		return err
	}
	if remoteURLsEqual(cur, wantURL) {
		return nil
	}
	if err := e.backend.RemoveRemote(ctx, dir, name); err != nil {
		return err
	}
	return e.backend.AddRemote(ctx, dir, name, wantURL)
}

// remoteURLsEqual 比较 remote URL:忽略末尾斜杠,避免无意义的重建。
func remoteURLsEqual(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

// defaultBranchOf 仓库默认分支,空则回退 "main"。
func defaultBranchOf(repo *model.Repo) string {
	if repo != nil && repo.DefaultBranch != "" {
		return repo.DefaultBranch
	}
	return model.DefaultBranch
}

func (e *Executor) push(ctx context.Context, dir string, task *model.SyncTask, repo *model.Repo, platform *model.Platform) error {
	// 必须用完整 refspec:go-git 按全名严格匹配本地 ref,不做 git CLI 的
	// 短名展开,"main:main" 匹配不到 refs/heads/main,会静默零推送
	// (返回 already up-to-date,被误判成功)。
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", task.SourceBranch, task.TargetBranch)

	_, err := e.backend.Push(ctx, gitbackend.PushOptions{
		RepoPath: dir,
		Remote:   RemoteTarget,
		RefSpecs: []string{refSpec},
		Force:    task.GitForce,
		Auth:     e.authConfig(ctx, repo, platform),
	})
	return err
}

func timePtr(t time.Time) *time.Time {
	return &t
}
