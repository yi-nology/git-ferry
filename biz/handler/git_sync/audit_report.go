package git_sync

import (
	"context"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/hertz/render"
	"github.com/oekit/pkg/ptrx"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 审计/合规报告导出 =====

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
	var req ops.AuditReportReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := int(req.Limit)
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
		Action:    ptrx.Deref(req.Action, ""),
		StartDate: ptrx.Deref(req.StartDate, ""),
		EndDate:   ptrx.Deref(req.EndDate, ""),
	}
	logs, total, err := svc.ListOperations(ctx, 0, int(limit), &filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if ptrx.Deref(req.Format, "") == "csv" {
		c.Render(consts.StatusOK, render.CSV{Rows: buildAuditCSVRows(logs)})
		return
	}
	response.Success(c, map[string]any{
		"items": logs,
		"total": total,
	})
}

// buildAuditCSVRows 构造审计报告 CSV 行(首行表头)。
// 交给 render.CSV 走标准库 encoding/csv 序列化(RFC 4180),单元格转义由库负责。
func buildAuditCSVRows(logs []*corebridge.OperationLog) [][]string {
	rows := make([][]string, 0, len(logs)+1)
	rows = append(rows, []string{"id", "action", "resource_type", "resource_key", "actor", "ip", "status", "created_at"})
	for _, l := range logs {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(l.ID), 10),
			l.Action, l.ResourceType, l.ResourceKey,
			l.Actor, l.IP, l.Status,
			l.CreatedAt.Format(time.RFC3339),
		})
	}
	return rows
}
