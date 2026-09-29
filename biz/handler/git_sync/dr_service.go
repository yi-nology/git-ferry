package git_sync

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// RunDRDrillReq 灾备演练请求。
type RunDRDrillReq struct {
	Name string `json:"name" form:"name" query:"name"`
	// All=true 时演练目录下全部(或最近 max 个)bundle;否则仅 Name。
	All bool `json:"all" form:"all" query:"all"`
	Max int  `json:"max" form:"max" query:"max"`
}

// RunDRDrill POST /api/v1/ops/dr-drill
// 从冷备 bundle 恢复到临时目录 → git fsck → refs 比对 → 出 RTO 报告。
func RunDRDrill(ctx context.Context, c *app.RequestContext) {
	var req RunDRDrillReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if req.All {
		max := req.Max
		if max <= 0 {
			max = 5
		}
		reports, summary, err := svc.BatchDRDrill(ctx, nil, max)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		recordAudit(ctx, c, "dr_drill_batch", "backup", "*", "灾备演练(批量)")
		response.Success(c, map[string]any{
			"mode":    "batch",
			"reports": reports,
			"summary": summary,
		})
		return
	}
	if req.Name == "" {
		response.BadRequest(c, "name is required (or set all=true)")
		return
	}
	rep, err := svc.RunDRDrill(ctx, req.Name)
	if err != nil && rep == nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "dr_drill", "backup", req.Name, "灾备演练")
	response.Success(c, map[string]any{
		"mode":    "single",
		"reports": []any{rep},
	})
}

// DrillHistory GET /api/v1/ops/dr-drill/history?limit=
func DrillHistory(ctx context.Context, c *app.RequestContext) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 20
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	entries, err := svc.DrillHistory(limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"items": entries,
		"total": len(entries),
	})
}

// VerifyDrillChain GET /api/v1/ops/dr-drill/chain/verify
func VerifyDrillChain(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	okChain, checked, broken := svc.VerifyDrillChain()
	response.Success(c, map[string]any{
		"ok":      okChain,
		"checked": checked,
		"broken":  broken,
	})
}

// BuildBackupManifest POST /api/v1/ops/backup-manifest
func BuildBackupManifest(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	m, err := svc.BuildBackupManifest()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "backup_manifest_build", "backup", "", "生成冷备完整性清单")
	response.Success(c, m)
}

// VerifyBackupManifest GET /api/v1/ops/backup-manifest/verify
func VerifyBackupManifest(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.VerifyBackupManifest()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, res)
}

// RPOReport GET /api/v1/ops/rpo?max_seconds=
func RPOReport(ctx context.Context, c *app.RequestContext) {
	maxSec, _ := strconv.ParseInt(c.Query("max_seconds"), 10, 64)
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	rep, err := svc.RPOReport(maxSec)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rep)
}
