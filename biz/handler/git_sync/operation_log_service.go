package git_sync

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/concurrency/async"
	operation_log "github.com/yi-nology/git-ferry/biz/model/operation_log"
	"github.com/yi-nology/git-ferry/internal/converter"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ListOperationLogs GET /api/v1/logs/operations
// 返回 {list, pagination, stats}（stats.total 为全量总数，pagination.total 为过滤后的结果数）。
func ListOperationLogs(ctx context.Context, c *app.RequestContext) {
	var req operation_log.ListOperationLogsReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	offset, limit := converter.PageToOffset(req.Page, req.PageSize)

	// 校验日期格式,避免非法字符串导致 DB 查询报错
	if req.StartDate != "" {
		if _, err := time.Parse("2006-01-02", req.StartDate); err != nil {
			response.BadRequest(c, "invalid start_date format, expected YYYY-MM-DD")
			return
		}
	}
	if req.EndDate != "" {
		if _, err := time.Parse("2006-01-02", req.EndDate); err != nil {
			response.BadRequest(c, "invalid end_date format, expected YYYY-MM-DD")
			return
		}
	}

	filter := corebridge.OperationLogFilter{
		Search:    req.Search,
		Action:    req.Action,
		Actor:     req.User,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	// ListOperations 与 OperationStats 互不依赖,并行执行缩短响应时间。
	// 旁路统计收编 oekit async.Spawn+Handle(concurrency#197 落地物):脱离请求
	// 生命周期(WithoutCancel 语义内置)+panic 收编进结果+泛型结果类型,
	// 手拼 channel+GoSafe+WithoutCancel+超时五件套删除。
	type statsResult struct {
		today, week, total int64
	}
	svc := GetSyncService()
	statsHandle := async.Spawn(ctx, "OperationStats", func(sctx context.Context) (statsResult, error) {
		today, week, total, err := svc.OperationStats(sctx)
		return statsResult{today: today, week: week, total: total}, err
	})

	list, total, err := svc.ListOperations(ctx, offset, limit, &filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 旁路统计限时 join:最多等 5s(超时任务继续跑完、迟到结果丢弃——
	// Spawn 语义显式化);请求 ctx 取消也提前放弃等待。
	sr, serr, timedOut := statsHandle.WaitUpTo(ctx, 5*time.Second)
	if serr != nil {
		response.InternalError(c, serr.Error())
		return
	}
	if timedOut {
		response.InternalError(c, "operation stats timed out after 5s")
		return
	}
	today, week, statsTotal := sr.today, sr.week, sr.total

	totalPages := int32(0)
	if req.PageSize > 0 && total > 0 {
		totalPages = converter.SafeInt64ToInt32((total + int64(req.PageSize) - 1) / int64(req.PageSize))
	}

	response.Success(c, &operation_log.ListOperationLogsResp{
		List: converter.ToOperationLogList(list),
		Pagination: &operation_log.OperationLogPagination{
			Page:       req.Page,
			PageSize:   req.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
		Stats: &operation_log.OperationLogStats{
			Today: today,
			Week:  week,
			Total: statsTotal,
		},
	})
}
