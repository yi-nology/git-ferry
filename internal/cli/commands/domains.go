package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/oekit/pkg/stringx"
	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// ---------- repo ----------

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "repo", Short: "仓库管理"}
	cmd.AddCommand(
		sc("+list", "仓库列表", repoList),
		sc("+info", "仓库详情", repoInfo),
		sc("+branches", "仓库分支", repoBranches),
		sc("+test", "测试仓库连通性（危险）", repoTest),
		sc("+create", "创建仓库记录（写）", repoCreate),
		sc("+delete", "删除仓库记录（危险）", repoDelete),
	)
	return cmd
}

func repoList(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	// 服务端 ListReposReq 的查询参数叫 search(不是 keyword)。
	// 传 keyword 会被服务端忽略 ⇒ --keyword 看着传了,其实没过滤。
	setIf(cmd, q, "keyword", "search")
	setIf(cmd, q, "platform", "platform")
	if flagBool(cmd, "all") {
		items, meta, err := c.PaginateAll("/api/v1/repos", q)
		if err != nil {
			return nil, err
		}
		// **总数用服务端给的**:自算 len(items) 会把"被截断的 10 条"谎报成总数。
		return output.Success(jsonRawArray(items), meta), nil
	}
	setIf(cmd, q, "page", "page")
	setIf(cmd, q, "limit", "page_size")
	return c.Get("/api/v1/repos", q)
}

func repoInfo(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/repo", url.Values{"key": {key}})
}

func repoBranches(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/repo/branches", url.Values{"key": {key}})
}

func repoTest(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("测试仓库连通性 key=" + key); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/repo/test", map[string]string{"key": key})
}

func repoCreate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	platform, err := requireFlag(cmd, "platform")
	if err != nil {
		return nil, err
	}
	name, err := requireFlag(cmd, "name")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("创建仓库记录 " + platform + "/" + name); denied != nil {
		return denied, nil
	}
	// 服务端的 CreateRepoReq 只有 name + remote_url;平台走 **query 的 platform_id**。
	//
	// 以前这里发的是 platform / clone_url / key —— 三个字段服务端都不认
	// (绑定时被静默丢弃)⇒ RemoteUrl 为空 ⇒ 400 ⇒ **该命令从来没能成功过**。
	remoteURL := flagStr(cmd, "clone-url")
	if remoteURL == "" {
		return nil, fmt.Errorf("--clone-url 不能为空(仓库的克隆地址)")
	}
	if v := flagStr(cmd, "key"); v != "" {
		return nil, fmt.Errorf("服务端不支持自定义仓库 key(按平台与仓库地址推导)")
	}
	platformID, err := resolvePlatformID(c, platform)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":       name,
		"remote_url": remoteURL,
	}
	q := url.Values{}
	q.Set("platform_id", platformID)
	return c.Do(http.MethodPost, "/api/v1/repo/create", body, q)
}

// resolvePlatformID 把平台 key 解析成服务端要的 platform_id。
//
// 服务端建仓接口要的是 **platform_id**(query),而 CLI 的 --platform 给的是
// 平台 key ⇒ 必须先查一次平台列表做映射,否则把 key 直接发过去服务端拿不到 ID。
func resolvePlatformID(c *client.Client, key string) (string, error) {
	env, err := c.Get("/api/v1/platforms", nil)
	if err != nil {
		return "", err
	}
	if !env.OK {
		return "", fmt.Errorf("查询平台列表失败: %s", env.Error.Message)
	}
	for _, raw := range client.ExtractItems(env.Data) {
		var p struct {
			ID  int64  `json:"id"`
			Key string `json:"key"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.Key == key {
			return strconv.FormatInt(p.ID, 10), nil
		}
	}
	return "", fmt.Errorf("找不到平台 %q(用 `gitferry platform +list` 看可用的 key)", key)
}

func repoDelete(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("删除仓库记录 key=" + key); denied != nil {
		return denied, nil
	}
	// 服务端为 POST /repo/delete?key=
	return c.Do("POST", "/api/v1/repo/delete", nil, url.Values{"key": {key}})
}

// ---------- task ----------

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "同步任务"}
	cmd.AddCommand(
		sc("+list", "任务列表", taskList),
		sc("+info", "任务详情", taskInfo),
		sc("+create", "创建任务（写）", taskCreate),
		sc("+update", "更新任务（写）", taskUpdate),
		sc("+run", "立即执行一次同步（危险）", taskRun),
		sc("+batch-run", "批量执行任务（危险，支持 --dry-run）", taskBatchRun),
		sc("+delete", "删除任务（危险）", taskDelete),
		sc("+preview", "同步预览（按源/目标仓库分支）", taskPreview),
	)
	return cmd
}

func taskList(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	// 服务端 ListTasksReq 有 repo_key / page / page_size / status 四个字段,
	// 这里一一对应。**status 是真的过滤条件**(服务端 ListTasksFiltered):
	// 之前契约里没有 status ⇒ hertz 静默忽略 ⇒ 用户以为筛过了、拿到的却是全量。
	setIf(cmd, q, "keyword", "repo_key")
	setIf(cmd, q, "status", "status")
	if flagBool(cmd, "all") {
		items, meta, err := c.PaginateAll("/api/v1/sync/tasks", q)
		if err != nil {
			return nil, err
		}
		return output.Success(jsonRawArray(items), meta), nil
	}
	setIf(cmd, q, "page", "page")
	setIf(cmd, q, "limit", "page_size")
	return c.Get("/api/v1/sync/tasks", q)
}

func taskInfo(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/sync/task", url.Values{"key": {key}})
}

func taskCreate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	name, err := requireFlag(cmd, "name")
	if err != nil {
		return nil, err
	}
	srcRepo, err := requireFlag(cmd, "source-repo")
	if err != nil {
		return nil, err
	}
	dstRepo, err := requireFlag(cmd, "target-repo")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("创建同步任务 " + name); denied != nil {
		return denied, nil
	}
	body := map[string]any{
		"name":            name,
		"source_repo_key": srcRepo,
		"source_branch":   flagOr(cmd, "source-branch", "main"),
		"target_repo_key": dstRepo,
		"target_branch":   flagOr(cmd, "target-branch", "main"),
		"sync_mode":       flagOr(cmd, "sync-mode", "all"),
		"cron":            flagStr(cmd, "cron"),
		"git_tags":        flagBool(cmd, "git-tags"),
		"git_force":       flagBool(cmd, "git-force"),
		"git_lfs":         flagBool(cmd, "git-lfs"),
		"git_push_prune":  flagBool(cmd, "git-push-prune"),
	}
	if v := flagStr(cmd, "include-branches"); v != "" {
		body["include_branches"] = v
	}
	if v := flagStr(cmd, "exclude-ref-patterns"); v != "" {
		body["exclude_ref_patterns"] = v
	}
	if v := flagStr(cmd, "force-push-policy"); v != "" {
		body["force_push_policy"] = v
	}
	return c.Post("/api/v1/sync/task/create", body)
}

func taskUpdate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("更新同步任务 key=" + key); denied != nil {
		return denied, nil
	}
	body := map[string]any{"key": key}
	for _, pair := range [][2]string{
		{"name", "name"},
		{"source-branch", "source_branch"},
		{"target-branch", "target_branch"},
		{"sync-mode", "sync_mode"},
		{"cron", "cron"},
	} {
		if v := flagStr(cmd, pair[0]); v != "" {
			body[pair[1]] = v
		}
	}
	if cmd.Flags().Changed("enabled") {
		body["enabled"] = flagBool(cmd, "enabled")
	}
	return c.Post("/api/v1/sync/task/update", body)
}

func taskRun(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("立即执行同步任务 key=" + key); denied != nil {
		return denied, nil
	}
	// 服务端:POST /api/v1/sync/task/run?key=
	return c.Do("POST", "/api/v1/sync/task/run", nil, url.Values{"key": {key}})
}

// batchRow 批量操作结果行。
type batchRow struct {
	TaskKey string `json:"task_key"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

// taskBatchRun 批量触发：--task-keys a,b,c；--dry-run 只列出计划；--csv 输出结果表。
func taskBatchRun(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	keys := stringx.SplitCSVNonEmpty(flagStr(cmd, "task-keys"))
	if len(keys) == 0 {
		if v := flagStr(cmd, "task"); v != "" {
			keys = []string{v}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("缺少 --task-keys a,b,c")
	}

	dry := flagBool(cmd, "dry-run")
	rows := make([]batchRow, 0, len(keys))
	okCount, failCount := 0, 0

	if dry {
		for _, k := range keys {
			rows = append(rows, batchRow{TaskKey: k, Action: "run", Status: "planned"})
			okCount++
		}
	} else {
		if denied := confirmDanger("批量执行 " + itoa(len(keys)) + " 个同步任务"); denied != nil {
			return denied, nil
		}
		for _, k := range keys {
			env, err := c.Do("POST", "/api/v1/sync/task/run", nil, url.Values{"key": {k}})
			r := batchRow{TaskKey: k, Action: "run"}
			switch {
			case err != nil:
				r.Status, r.Error = "failed", err.Error()
				failCount++
			case !env.OK:
				r.Status, r.Error = "failed", env.Error.Message
				failCount++
			default:
				r.Status = "ok"
				okCount++
			}
			rows = append(rows, r)
		}
	}

	payload := map[string]any{
		"dry_run":   dry,
		"total":     len(keys),
		"succeeded": okCount,
		"failed":    failCount,
		"results":   rows,
	}
	if flagBool(cmd, "csv") {
		payload["csv"] = batchResultsCSV(rows)
	}
	env := output.Success(payload, nil)
	if failCount > 0 {
		// **部分失败 ⇒ 退出码非 0**:以前一律 ok:true ⇒ 脚本里
		// `gitferry task +batch-run ... && echo ok` 会打印 ok,而实际 3/3 全失败
		// (失败数被埋在 data 的 "failed" 字段里)。
		env.OK = false
		env.Error = &output.ErrorInfo{
			Code:    "batch_failed",
			Message: fmt.Sprintf("%d/%d 个任务失败", failCount, len(keys)),
		}
	}
	return env, nil
}

func batchResultsCSV(rows []batchRow) string {
	esc := func(s string) string {
		// 最小 CSV 转义：引号/逗号/换行
		if strings.ContainsAny(s, ",\"\n\r") {
			return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		return s
	}
	var b strings.Builder
	b.WriteString("task_key,action,status,error\n")
	for _, r := range rows {
		b.WriteString(esc(r.TaskKey) + "," + esc(r.Action) + "," + esc(r.Status) + "," + esc(r.Error) + "\n")
	}
	return b.String()
}

func taskDelete(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("删除同步任务 key=" + key); denied != nil {
		return denied, nil
	}
	return c.Do("POST", "/api/v1/sync/task/delete", nil, url.Values{"key": {key}})
}

func taskPreview(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	srcRepo, err := requireFlag(cmd, "source-repo")
	if err != nil {
		return nil, err
	}
	dstRepo, err := requireFlag(cmd, "target-repo")
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/sync/preview", map[string]any{
		"source_repo_key": srcRepo,
		"source_branch":   flagOr(cmd, "source-branch", "main"),
		"target_repo_key": dstRepo,
		"target_branch":   flagOr(cmd, "target-branch", "main"),
	})
}

// ---------- history ----------

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "history", Short: "同步执行历史与诊断"}
	cmd.AddCommand(
		sc("+list", "执行历史列表（含步骤链）", historyList),
		sc("+detail", "单次执行详情（按 run-id 过滤）", historyDetail),
		sc("+diagnose", "失败诊断：错误分类→原因→建议", historyDiagnose),
		sc("+retry", "重试失败执行（危险）", historyRetry),
	)
	return cmd
}

func historyList(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	setIf(cmd, q, "task", "task_key")
	setIf(cmd, q, "limit", "limit")
	return c.Get("/api/v1/sync/history", q)
}

func historyDetail(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	runID, err := requireFlag(cmd, "run-id")
	if err != nil {
		return nil, err
	}
	q := url.Values{"run_id": {runID}}
	setIf(cmd, q, "task", "task_key")
	return c.Get("/api/v1/sync/history", q)
}

func historyDiagnose(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	runID, err := requireFlag(cmd, "run-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/ops/diagnose", url.Values{"run_id": {runID}})
}

func historyRetry(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	runID, err := requireFlag(cmd, "run-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("重试同步执行 run_id=" + runID); denied != nil {
		return denied, nil
	}
	n, convErr := strconv.Atoi(runID)
	if convErr != nil {
		return nil, fmt.Errorf("--run-id 必须是数字，实际 %q", runID)
	}
	return c.Post("/api/v1/ops/retry", map[string]any{"run_id": n})
}

// ---------- ops ----------

func newOpsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ops", Short: "运维中心：健康/盘点/RPO/漂移/审计/灾备"}
	cmd.AddCommand(
		sc("+overview", "系统概览", opsGet("/api/v1/ops/overview")),
		sc("+todo", "统一待办队列（健康/孤儿/RPO）", opsGet("/api/v1/ops/todo")),
		sc("+trends", "同步成功率/耗时趋势", opsTrends),
		sc("+git-url", "打印 Git Smart HTTP clone URL", opsGitURL),
		sc("+org-map", "按策略解析目标仓 key（preserve/single/flat/mixed）", opsOrgMap),
		sc("+health", "健康评分（--with-drift 折入漂移）", opsHealth),
		sc("+inventory", "资产盘点（孤儿仓库）", opsGet("/api/v1/ops/inventory")),
		sc("+rpo", "RPO/RTO 观测", opsRPO),
		sc("+drift", "漂移检测", opsDrift),
		sc("+integrity", "冷备完整性校验", opsGet("/api/v1/ops/backup-manifest/verify")),
		sc("+audit", "审计哈希链校验", opsGet("/api/v1/ops/audit-chain/verify")),
		sc("+audit-report", "审计日志导出（--csv 下载）", opsAuditReport),
		opsForcePushApprovalsCmd(),
		sc("+retry-batch", "批量重试失败任务（危险）", opsRetryBatch),
		sc("+drill", "灾备演练（危险）", opsDrill),
		sc("+rebuild", "任务全量重建（危险）", opsRebuild),
		opsPushBackupCmd(),
		opsRepoFilesCmd(),
		opsMetadataRestoreCmd(),
	)
	return cmd
}

// opsPushBackupCmd 注册 push-backup 自己的 flag。
// --remote 是这个命令的必填参数,之前没有注册 ⇒ 命令永远报"缺少必填参数 --remote";
// --refspec 同理没注册,会被 flagStr 静默吞掉(用户以为传了,其实没生效)。
func opsPushBackupCmd() *cobra.Command {
	c := sc("+push-backup", "推送到备份远端 github/gitlab", opsPushBackup)
	f := c.Flags()
	f.String("remote", "", "备份远端地址(https://…/repo.git 或 git@host:owner/repo.git)")
	f.String("refspec", "", "要推送的 refspec(默认 refs/heads/*:refs/heads/*)")
	return c
}

// opsRepoFilesCmd 注册 repo-files 自己的 flag。
// --path 没注册时会被 flagStr 静默吞掉,文件过滤形同虚设。
func opsRepoFilesCmd() *cobra.Command {
	c := sc("+repo-files", "浏览任务 workdir 文件", opsRepoFiles)
	c.Flags().String("path", "", "只显示该路径下的文件(前缀过滤)")
	return c
}

func opsMetadataRestoreCmd() *cobra.Command {
	c := sc("+metadata-restore", "元数据回灌（默认 dry-run；--execute 才写入）", opsMetadataRestore)
	f := c.Flags()
	f.String("kinds", "", "逗号分隔：labels,milestones,issues,prs,releases")
	f.String("target-owner", "", "目标 owner（默认同源）")
	// target-repo 已由 sc() 统一挂上，重复定义会 panic: flag redefined
	f.Bool("execute", false, "真正写入（缺省 dry-run 预览）")
	f.Bool("overwrite", false, "同名 label 覆盖更新")
	return c
}

func opsGet(path string) runFunc {
	return func(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
		q := url.Values{}
		setIf(cmd, q, "task", "task_key")
		setIf(cmd, q, "limit", "limit")
		return c.Get(path, q)
	}
}

// opsPost 走 POST 的 ops 端点。
// 有些 ops 端点只注册了 POST,CLI 用 GET 会拿到 404,命令看起来存在却永远调不通。

// opsDrift 漂移检测。
//
// **查询参数名与服务端一致**:服务端要的是 `task_keys`(复数数组),
// 以前发 `task_key`(单数)⇒ 被静默忽略 ⇒ TaskKeys 为空 ⇒ 服务端走
// **全量扫描**(用户要单任务报告,实际拿到全部任务的漂移)。
// drift 也没有 `limit` 参数(扫描范围由服务端预算决定)⇒ 不发。
func opsDrift(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	for _, k := range stringx.SplitCSVNonEmpty(flagStr(cmd, "task")) {
		q.Add("task_keys", k)
	}
	return c.Do(http.MethodPost, "/api/v1/ops/drift", nil, q)
}

// opsTrends 同步趋势（--limit 作 days）。
func opsTrends(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	if v := flagStr(cmd, "limit"); v != "" {
		q.Set("days", v)
	}
	return c.Get("/api/v1/ops/trends", q)
}

// opsGitURL 打印 git clone URL（需服务端启用 git_serve）。
func opsGitURL(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	// key 形如 platform/owner/repo 或 owner/repo；统一转 /git/<key>.git
	repoPath := strings.Trim(key, "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}
	clone := strings.TrimRight(c.BaseURL, "/") + "/git/" + repoPath
	return output.Success(map[string]any{
		"repo_key":  key,
		"clone_url": clone,
		"note":      "需服务端启用 git_serve.enabled；路径形如 /git/<host>/<owner>/<repo>.git",
	}, nil), nil
}

func opsHealth(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	setIf(cmd, q, "limit", "limit")
	if flagBool(cmd, "with-drift") {
		q.Set("with_drift", "1")
	}
	return c.Get("/api/v1/ops/health-score", q)
}

// opsAuditReport 审计导出；--csv 原样输出 CSV 文本（不包 Envelope）。
//
// **stdout 只能有 CSV**:之前 --csv 先把 CSV 打到 stdout,再让框架把
// Envelope JSON 也打到 stdout ⇒ `> out.csv` 得到的文件尾部多一段 JSON
// ⇒ CSV 解析器直接报错(导出文件是坏的)。
//
// 修法:CSV 模式下返回 nil Envelope,并让 runWith 看到 nil 就不再输出信封。
func opsAuditReport(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	setIf(cmd, q, "limit", "limit")
	setIf(cmd, q, "action", "action")
	if flagBool(cmd, "csv") {
		q.Set("format", "csv")
	}
	env, err := c.Do("GET", "/api/v1/ops/audit-report", nil, q)
	if err != nil {
		return nil, err
	}
	if !flagBool(cmd, "csv") {
		return env, nil
	}
	// 服务端出错(4xx/5xx)⇒ 原样输出失败信封,退出码非 0;别把错误当 CSV 打出去。
	if !env.OK {
		return env, nil
	}
	// 服务端 CSV 走 render.CSV ⇒ 响应体是**裸 CSV 文本**,不是 JSON 信封。
	raw, ok := env.Data.(json.RawMessage)
	if !ok {
		return env, nil
	}
	fmt.Print(string(raw))
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		fmt.Println()
	}
	// 已经自己输出原始内容了 ⇒ 不要再输出 Envelope。
	return nil, nil
}

func opsRPO(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	setIf(cmd, q, "max-seconds", "max_seconds")
	return c.Get("/api/v1/ops/rpo", q)
}

func opsRetryBatch(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if flagBool(cmd, "dry-run") {
		body := map[string]any{"dry_run": true, "action": "retry-batch"}
		if v := flagStr(cmd, "limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				body["limit"] = n
			}
		}
		if v := flagStr(cmd, "task"); v != "" {
			body["task_key"] = v
		}
		body["note"] = "dry-run：未实际重试；去掉 --dry-run 并加 --yes 执行"
		return output.Success(body, nil), nil
	}
	if denied := confirmDanger("批量重试失败执行"); denied != nil {
		return denied, nil
	}
	body := map[string]any{}
	if v := flagStr(cmd, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			body["limit"] = n
		}
	}
	if v := flagStr(cmd, "task"); v != "" {
		body["task_key"] = v
	}
	return c.Post("/api/v1/ops/retry-batch", body)
}

func opsDrill(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	name, err := requireFlag(cmd, "name")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("灾备演练 bundle=" + name); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/ops/dr-drill", map[string]any{"name": name})
}

func opsRebuild(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	task, err := requireFlag(cmd, "task")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("清空工作目录并全量重建 task=" + task); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/ops/rebuild", map[string]any{"task_key": task})
}

// opsMetadataRestore 元数据回灌：--key 仓库；--name 作 snapshot_dir；
// 默认 dry-run，真正写入需去掉 --dry-run 的反义（--execute）且加 --yes。
func opsMetadataRestore(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	repoKey, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"repo_key": repoKey}
	if v := flagStr(cmd, "name"); v != "" {
		body["snapshot_dir"] = v
	}
	if v := flagStr(cmd, "kinds"); v != "" {
		body["kinds"] = stringx.SplitCSVNonEmpty(v)
	}
	if v := flagStr(cmd, "target-owner"); v != "" {
		body["target_owner"] = v
	}
	if v := flagStr(cmd, "target-repo"); v != "" {
		body["target_repo"] = v
	}
	// dry-run 是默认安全路径；--execute 才写入
	if !flagBool(cmd, "execute") {
		body["dry_run"] = true
		body["note"] = "dry-run：未写入目标；确认后加 --execute --yes"
		return c.Post("/api/v1/ops/metadata-restore", body)
	}
	body["dry_run"] = false
	if flagBool(cmd, "overwrite") {
		body["overwrite"] = true
	}
	if denied := confirmDanger("元数据回灌 repo=" + repoKey); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/ops/metadata-restore", body)
}

// ---------- platform ----------

func newPlatformCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "platform", Short: "平台管理"}
	cmd.AddCommand(
		sc("+list", "平台列表", platformList),
		sc("+test", "测试平台连通性（危险）", platformTest),
		sc("+sync-repos", "从平台同步仓库清单（危险）", platformSyncRepos),
	)
	return cmd
}

func platformList(c *client.Client, _ *config.Config, _ *cobra.Command, _ []string) (*output.Envelope, error) {
	return c.Get("/api/v1/platforms", nil)
}

func platformTest(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("测试平台连通性 key=" + key); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/platform/test", map[string]any{"key": key})
}

func platformSyncRepos(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	key, err := requireFlag(cmd, "key")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("从平台拉取仓库清单 key=" + key); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/platform/sync-repos", map[string]any{"key": key})
}

// ---------- webhook ----------

func newWebhookCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "webhook", Short: "Webhook 规则与事件"}
	cmd.AddCommand(
		sc("+rules", "Webhook 规则列表", func(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
			q := url.Values{}
			// 服务端 ListRulesReq 要求 repo_key(缺了直接 400),CLI 必须带上。
			setIf(cmd, q, "key", "repo_key")
			return c.Get("/api/v1/webhook/rules", q)
		}),
		sc("+events", "Webhook 事件列表", func(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
			q := url.Values{}
			// 服务端 ListEventsReq 同样要求 repo_key(缺了 400)。
			setIf(cmd, q, "key", "repo_key")
			setIf(cmd, q, "limit", "limit")
			return c.Get("/api/v1/webhook/events", q)
		}),
	)
	return cmd
}

// ---------- helpers ----------

func flagOr(cmd *cobra.Command, name, def string) string {
	if v := flagStr(cmd, name); v != "" {
		return v
	}
	return def
}

func jsonRawArray(items []json.RawMessage) json.RawMessage {
	if items == nil {
		items = []json.RawMessage{}
	}
	b, _ := json.Marshal(items)
	return b
}

// itoa int → 字符串(统一走 core strutil,别在壳层再包一层 strconv)。
func itoa(n int) string { return textutil.Itoa(n) }

// opsOrgMap 调用 /ops/resolve-org-target 做 org 映射预览。
func opsOrgMap(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	src := flagStr(cmd, "source-repo")
	if src == "" {
		src = flagStr(cmd, "key")
	}
	if src == "" {
		return nil, fmt.Errorf("缺少 --source-repo（或 --key）")
	}
	body := map[string]any{
		"source_repo_key": src,
		"org_mapping":     flagOr(cmd, "org-mapping", "preserve"),
		"target_org":      flagStr(cmd, "target-org"),
		"target_platform": flagStr(cmd, "platform"),
		"is_personal":     flagBool(cmd, "personal"),
	}
	return c.Post("/api/v1/ops/resolve-org-target", body)
}

func opsPushBackup(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	task, err := requireFlag(cmd, "task")
	if err != nil {
		return nil, err
	}
	remote, err := requireFlag(cmd, "remote")
	if err != nil {
		return nil, err
	}
	if flagBool(cmd, "dry-run") {
		return c.Post("/api/v1/ops/push-backup", map[string]any{
			"task_key": task, "remote": remote, "dry_run": true,
		})
	}
	if denied := confirmDanger("推送备份到 " + remote + " task=" + task); denied != nil {
		return denied, nil
	}
	body := map[string]any{"task_key": task, "remote": remote, "force": flagBool(cmd, "git-force")}
	if v := flagStr(cmd, "refspec"); v != "" {
		body["refspec"] = v
	}
	return c.Post("/api/v1/ops/push-backup", body)
}

func opsRepoFiles(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	task, err := requireFlag(cmd, "task")
	if err != nil {
		return nil, err
	}
	q := url.Values{"task_key": {task}}
	if p := flagStr(cmd, "path"); p != "" {
		q.Set("path", p)
	}
	return c.Get("/api/v1/ops/repo-files", q)
}

// opsForcePushApprovalsCmd 强推审批的列表/放行/撤销。
//
// 审批是"人工授权覆盖目标侧独有提交"：批了要能**收回**（revoke）——
// 没有撤销口子的话唯一的取消办法是等它被用掉，而那正是最不该覆盖的时刻。
func opsForcePushApprovalsCmd() *cobra.Command {
	c := sc("+force-push-approvals", "强推审批列表/放行/撤销", opsForcePushApprovals)
	f := c.Flags()
	f.String("approve", "", "放行指定审批 ID")
	f.String("revoke", "", "撤销指定审批 ID")
	return c
}

func opsForcePushApprovals(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if id := flagStr(cmd, "approve"); id != "" {
		if denied := confirmDanger("放行强推审批 " + id); denied != nil {
			return denied, nil
		}
		return c.Post("/api/v1/ops/force-push-approvals/approve", map[string]any{"id": id})
	}
	if id := flagStr(cmd, "revoke"); id != "" {
		if denied := confirmDanger("撤销强推审批 " + id); denied != nil {
			return denied, nil
		}
		return c.Post("/api/v1/ops/force-push-approvals/revoke", map[string]any{"id": id})
	}
	q := url.Values{}
	setIf(cmd, q, "task", "task_key")
	return c.Get("/api/v1/ops/force-push-approvals", q)
}
