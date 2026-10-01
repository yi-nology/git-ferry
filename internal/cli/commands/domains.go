package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
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
	setIf(cmd, q, "keyword", "keyword")
	setIf(cmd, q, "platform", "platform")
	if flagBool(cmd, "all") {
		items, err := c.PaginateAll("/api/v1/repos", q)
		if err != nil {
			return nil, err
		}
		return output.Success(jsonRawArray(items), &output.Meta{TotalCount: len(items)}), nil
	}
	setIf(cmd, q, "page", "page")
	setIf(cmd, q, "limit", "per_page")
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
	body := map[string]any{
		"platform":  platform,
		"name":      name,
		"clone_url": flagStr(cmd, "clone-url"),
	}
	if v := flagStr(cmd, "key"); v != "" {
		body["key"] = v
	}
	return c.Post("/api/v1/repo/create", body)
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
	setIf(cmd, q, "keyword", "keyword")
	setIf(cmd, q, "status", "status")
	if flagBool(cmd, "all") {
		items, err := c.PaginateAll("/api/v1/sync/tasks", q)
		if err != nil {
			return nil, err
		}
		return output.Success(jsonRawArray(items), &output.Meta{TotalCount: len(items)}), nil
	}
	setIf(cmd, q, "page", "page")
	setIf(cmd, q, "limit", "limit")
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
		"git_prune":       flagBool(cmd, "git-prune"),
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
	keys := splitCSV(flagStr(cmd, "task-keys"))
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
	return output.Success(payload, nil), nil
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
		sc("+health", "健康评分（--with-drift 折入漂移）", opsHealth),
		sc("+inventory", "资产盘点（孤儿仓库）", opsGet("/api/v1/ops/inventory")),
		sc("+rpo", "RPO/RTO 观测", opsRPO),
		sc("+drift", "漂移检测", opsGet("/api/v1/ops/drift")),
		sc("+integrity", "冷备完整性校验", opsGet("/api/v1/ops/backup-manifest/verify")),
		sc("+audit", "审计哈希链校验", opsGet("/api/v1/ops/audit-chain/verify")),
		sc("+audit-report", "审计日志导出（--csv 下载）", opsAuditReport),
		sc("+retry-batch", "批量重试失败任务（危险）", opsRetryBatch),
		sc("+drill", "灾备演练（危险）", opsDrill),
		sc("+rebuild", "任务全量重建（危险）", opsRebuild),
		opsMetadataRestoreCmd(),
	)
	return cmd
}

func opsMetadataRestoreCmd() *cobra.Command {
	c := sc("+metadata-restore", "元数据回灌（默认 dry-run；--execute 才写入）", opsMetadataRestore)
	f := c.Flags()
	f.String("kinds", "", "逗号分隔：labels,milestones,issues,prs,releases")
	f.String("target-owner", "", "目标 owner（默认同源）")
	f.String("target-repo", "", "目标仓库名（默认同源）")
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

func opsHealth(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	q := url.Values{}
	setIf(cmd, q, "limit", "limit")
	if flagBool(cmd, "with-drift") {
		q.Set("with_drift", "1")
	}
	return c.Get("/api/v1/ops/health-score", q)
}

// opsAuditReport 审计导出；--csv 原样输出 CSV 文本（不包 Envelope）。
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
	// CSV：把 data 当原文写到 stdout
	if raw, ok := env.Data.(json.RawMessage); ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			fmt.Print(s)
			if !strings.HasSuffix(s, "\n") {
				fmt.Println()
			}
			return output.Success(map[string]any{"exported": true, "format": "csv"}, nil), nil
		}
		fmt.Print(string(raw))
		return output.Success(map[string]any{"exported": true, "format": "csv"}, nil), nil
	}
	return env, nil
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
		body["kinds"] = splitCSV(v)
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
		sc("+rules", "Webhook 规则列表", func(c *client.Client, _ *config.Config, _ *cobra.Command, _ []string) (*output.Envelope, error) {
			return c.Get("/api/v1/webhook/rules", nil)
		}),
		sc("+events", "Webhook 事件列表", func(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
			q := url.Values{}
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

func itoa(n int) string { return strconv.Itoa(n) }
