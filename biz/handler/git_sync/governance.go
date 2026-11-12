package git_sync

import (
	"context"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== P3 治理能力透出:对账 / 恢复点 / 策略即代码 =====
//
// 走 custom.go 手写路由(与 ops 系一致),不进 IDL/hz 生成——
// 避开 hz 生成 tag 风格回归(见 CHANGELOG Notes 的手改坑)。

// ReconcileTask GET /api/v1/ops/reconcile?task_key=
// 手动触发单任务跨平台一致性对账(纯平台 API 只读)。
func ReconcileTask(ctx context.Context, c *app.RequestContext) {
	taskKey := c.Query("task_key")
	if taskKey == "" {
		response.BadRequest(c, "task_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.ReconcileTask(ctx, taskKey)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, res)
}

// ReconcileResults GET /api/v1/ops/reconcile/results?all=
// 对账结果列表(默认只列漂移/缺失=all 空;all=1 列全部)。
func ReconcileResults(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if c.Query("all") == "1" {
		list, err := svc.ListReconcileAll(ctx)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.Success(c, map[string]any{"items": list, "total": len(list)})
		return
	}
	drift, err := svc.ListReconcileDrift(ctx)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"items": drift, "total": len(drift)})
}

// RecoveryPoints GET /api/v1/ops/recovery-points?task_key=&limit=
// 恢复点索引(最新优先;文件已轮转的条目惰性清理)。
func RecoveryPoints(ctx context.Context, c *app.RequestContext) {
	taskKey := c.Query("task_key")
	if taskKey == "" {
		response.BadRequest(c, "task_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	limit := 0
	if v := c.Query("limit"); v != "" {
		n, aerr := strconv.Atoi(v)
		if aerr != nil || n < 0 {
			response.BadRequest(c, "invalid limit")
			return
		}
		limit = n
	}
	list, err := svc.ListRecoveryPoints(taskKey, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"items": list, "total": len(list)})
}

// ExportPolicies GET /api/v1/ops/policies/export
// 全量任务策略导出(确定性 JSON,可进 git 走 PR 审阅)。
func ExportPolicies(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	doc, err := svc.ExportTaskPolicies(ctx)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// 直接给 JSON 文档(带 Content-Disposition 方便另存)。
	c.Header("Content-Disposition", `attachment; filename="gitferry-policies.json"`)
	c.SetContentTypeBytes([]byte("application/json; charset=utf-8"))
	c.SetStatusCode(http.StatusOK)
	_, _ = c.WriteString(doc)
}

// ApplyPolicies POST /api/v1/ops/policies/apply
// 策略回放:body = 策略文件内容(与 export 同 schema)。
// 单任务失败不中断其余,结果完整报告(created/updated/errors)。
func ApplyPolicies(ctx context.Context, c *app.RequestContext) {
	body := c.Request.Body()
	if len(body) == 0 {
		response.BadRequest(c, "policy document body is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.ApplyTaskPolicies(ctx, body)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, res)
}
