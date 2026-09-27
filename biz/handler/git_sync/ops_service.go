package git_sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/health"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 失败补偿:手动/批量重试 =====

// RetryRunReq 按执行记录重跑对应任务。
type RetryRunReq struct {
	RunID uint `json:"run_id" form:"run_id" query:"run_id"`
}

// RetryRun 手动重试一条失败执行:取 run 的 task_key 再触发一次同步。
// 只允许重试 failed 状态,避免把成功任务误触发。
func RetryRun(ctx context.Context, c *app.RequestContext) {
	var req RetryRunReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RunID == 0 {
		response.BadRequest(c, "run_id is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	run, taskKey, err := findRunByID(ctx, svc, req.RunID)
	if err != nil {
		response.NotFound(c, "run not found")
		return
	}
	if run.Status != "failed" {
		response.BadRequest(c, fmt.Sprintf("only failed runs can be retried, got status %q", run.Status))
		return
	}
	if err := svc.RunTaskAsync(taskKey, "manual_retry", nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "retry", "sync_run", fmt.Sprint(req.RunID), "手动重试同步 "+taskKey)
	response.Success(c, map[string]any{
		"success":  true,
		"message":  "retry started",
		"task_key": taskKey,
	})
}

// BatchRetryReq 批量重试最近失败的执行。
type BatchRetryReq struct {
	// Limit 最多重试多少条;默认 10,上限 50
	Limit int `json:"limit" form:"limit" query:"limit"`
	// TaskKey 仅重试该任务的失败
	TaskKey string `json:"task_key" form:"task_key" query:"task_key"`
}

// BatchRetryFailed 扫描最近失败执行并批量重跑(带预览语义:返回将要/已重试列表)。
func BatchRetryFailed(ctx context.Context, c *app.RequestContext) {
	var req BatchRetryReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	failed, err := collectRecentFailedRuns(ctx, svc, req.TaskKey, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	retried := make([]map[string]any, 0, len(failed))
	skipped := 0
	for _, item := range failed {
		if err := svc.RunTaskAsync(item.taskKey, "batch_retry", nil); err != nil {
			skipped++
			slog.Warn("batch retry skip", "task", item.taskKey, "error", err)
			continue
		}
		retried = append(retried, map[string]any{
			"run_id":   item.runID,
			"task_key": item.taskKey,
		})
	}
	recordAudit(ctx, c, "batch_retry", "sync_run", req.TaskKey,
		fmt.Sprintf("批量重试 %d 条失败执行", len(retried)))
	response.Success(c, map[string]any{
		"success":   true,
		"retried":   retried,
		"skipped":   skipped,
		"candidate": len(failed),
	})
}

// ===== P1:仓库健康评分 =====

// HealthScoreItem 单仓库/任务健康分。
type HealthScoreItem struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Score  int      `json:"score"` // 0-100
	Level  string   `json:"level"` // gold/silver/bronze/basic
	Issues []string `json:"issues,omitempty"`
}

// HealthScoreReq 评分请求。
type HealthScoreReq struct {
	Limit int `json:"limit" form:"limit" query:"limit"`
}

// HealthScore 按规则给任务打分(借鉴 Port Scorecards 的等级模型)。
// 规则(壳层可计算,不依赖 core 扩展):
//   - 有最近成功执行 (+40)
//   - 最近执行无错误 (+30)
//   - 有 cron 或 webhook 触发配置 (+20)
//   - 有任务名/描述元数据 (+10)
func HealthScore(ctx context.Context, c *app.RequestContext) {
	var req HealthScoreReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	items, err := computeHealthScores(ctx, svc, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// ===== P1:同步概览仪表盘 =====

// SyncOverview 同步侧仪表盘指标。
func SyncOverview(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	taskStatus, err := svc.CountTasksByStatus()
	if err != nil {
		response.InternalError(c, "count tasks failed")
		return
	}
	repoCount, err := svc.CountRepos()
	if err != nil {
		response.InternalError(c, "count repos failed")
		return
	}
	failed, _ := collectRecentFailedRuns(ctx, svc, "", 20)
	response.Success(c, map[string]any{
		"repo_count":      repoCount,
		"tasks_by_status": taskStatus,
		"recent_failed":   failed,
		"health":          svc.HealthCheck(),
		"generated_at":    time.Now().Format(time.RFC3339),
	})
}

// ===== 内部工具 =====

type failedRun struct {
	runID    uint
	taskKey  string
	errorMsg string
	errType  string
	endAt    time.Time
}

func findRunByID(ctx context.Context, svc *corebridge.Service, runID uint) (*corebridge.SyncRun, string, error) {
	const pageSize = 50
	offset := 0
	for page := 0; page < 10; page++ {
		tasks, total, err := svc.ListTasks(ctx, "", offset, pageSize)
		if err != nil {
			return nil, "", err
		}
		for _, t := range tasks {
			runs, _, err := svc.ListHistory(ctx, t.Key, 0, 50)
			if err != nil {
				continue
			}
			for _, r := range runs {
				if r.ID == runID {
					return r, t.Key, nil
				}
			}
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return nil, "", fmt.Errorf("run %d not found", runID)
}

func collectRecentFailedRuns(ctx context.Context, svc *corebridge.Service, taskKey string, limit int) ([]failedRun, error) {
	var out []failedRun
	const pageSize = 50
	offset := 0
	for page := 0; page < 10 && len(out) < limit; page++ {
		tasks, total, err := svc.ListTasks(ctx, taskKey, offset, pageSize)
		if err != nil {
			return out, err
		}
		for _, t := range tasks {
			runs, _, err := svc.ListHistory(ctx, t.Key, 0, 10)
			if err != nil {
				continue
			}
			for _, r := range runs {
				if r.Status != "failed" {
					continue
				}
				end := time.Now()
				if r.EndTime != nil {
					end = *r.EndTime
				}
				out = append(out, failedRun{
					runID: r.ID, taskKey: t.Key,
					errorMsg: r.ErrorMessage, errType: r.ErrorType, endAt: end,
				})
				if len(out) >= limit {
					return out, nil
				}
			}
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return out, nil
}

// healthRules 可替换(测试/后续由配置注入)。
var healthRules = health.DefaultConfig()

func computeHealthScores(ctx context.Context, svc *corebridge.Service, limit int) ([]HealthScoreItem, error) {
	tasks, _, err := svc.ListTasks(ctx, "", 0, limit)
	if err != nil {
		return nil, err
	}
	items := make([]HealthScoreItem, 0, len(tasks))
	for _, t := range tasks {
		facts := health.Facts{
			"has_name": boolFact(t.Name != ""),
			"has_cron": boolFact(t.Cron != ""),
		}
		runs, _, rerr := svc.ListHistory(ctx, t.Key, 0, 5)
		if rerr == nil && len(runs) > 0 {
			facts["has_history"] = "true"
			last := runs[0]
			facts["recent_success"] = boolFact(last.Status == "success")
			facts["error_free"] = boolFact(last.ErrorMessage == "")
		} else {
			facts["has_history"] = "false"
		}
		res := healthRules.Evaluate(facts)
		items = append(items, HealthScoreItem{
			Key: t.Key, Name: t.Name,
			Score: res.Score, Level: res.Level, Issues: res.Issues,
		})
	}
	return items, nil
}

func boolFact(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ===== P1:审计/合规报告导出 =====

// AuditReportReq 报告请求。
type AuditReportReq struct {
	StartDate string `json:"start_date" form:"start_date" query:"start_date"`
	EndDate   string `json:"end_date" form:"end_date" query:"end_date"`
	Action    string `json:"action" form:"action" query:"action"`
	Format    string `json:"format" form:"format" query:"format"` // json | csv
	Limit     int    `json:"limit" form:"limit" query:"limit"`
}

// AuditReport 导出操作日志,便于合规留档(策略变更/重试记录都在审计里)。
func AuditReport(ctx context.Context, c *app.RequestContext) {
	var req AuditReportReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	filter := corebridge.OperationLogFilter{
		Action:    req.Action,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}
	logs, total, err := svc.ListOperations(ctx, 0, limit, &filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if req.Format == "csv" {
		csv := buildAuditCSV(logs)
		c.Data(consts.StatusOK, "text/csv; charset=utf-8", []byte(csv))
		return
	}
	response.Success(c, map[string]any{
		"items": logs,
		"total": total,
	})
}

func buildAuditCSV(logs []*corebridge.OperationLog) string {
	var b strings.Builder
	b.WriteString("id,action,resource_type,resource_key,actor,ip,status,created_at\n")
	for _, l := range logs {
		fmt.Fprintf(&b, "%d,%s,%s,%s,%s,%s,%s,%s\n",
			l.ID, csvEscape(l.Action), csvEscape(l.ResourceType), csvEscape(l.ResourceKey),
			csvEscape(l.Actor), csvEscape(l.IP), csvEscape(l.Status),
			l.CreatedAt.Format(time.RFC3339))
	}
	return b.String()
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}
