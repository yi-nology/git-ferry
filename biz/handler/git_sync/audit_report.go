package git_sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
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
		Action:    optStr(req.Action),
		StartDate: optStr(req.StartDate),
		EndDate:   optStr(req.EndDate),
	}
	logs, total, err := svc.ListOperations(ctx, 0, int(limit), &filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if optStr(req.Format) == "csv" {
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
			l.ID, textutil.CSVEscape(l.Action), textutil.CSVEscape(l.ResourceType), textutil.CSVEscape(l.ResourceKey),
			textutil.CSVEscape(l.Actor), textutil.CSVEscape(l.IP), textutil.CSVEscape(l.Status),
			l.CreatedAt.Format(time.RFC3339))
	}
	return b.String()
}
