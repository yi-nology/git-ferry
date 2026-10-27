package git_sync

import (
	"context"
	"github.com/oekit/pkg/errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/pkg/convert"
	"github.com/oekit/pkg/ptrx"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// RunDRDrill POST /api/v1/ops/dr-drill
// 从冷备 bundle 恢复到临时目录 → git fsck → refs 比对 → 出 RTO 报告。
func RunDRDrill(ctx context.Context, c *app.RequestContext) {
	var req ops.RunDRDrillReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if ptrx.Deref(req.All, false) {
		max := int(req.Max)
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
	if ptrx.Deref(req.Name, "") == "" {
		response.BadRequest(c, "name is required (or set all=true)")
		return
	}
	rep, err := svc.RunDRDrill(ctx, ptrx.Deref(req.Name, ""))
	if err != nil && rep == nil {
		response.InternalError(c, err.Error())
		return
	}
	metaCheck := map[string]any{}
	if ptrx.Deref(req.WithMetadata, true) {
		rep, verr := svc.SampleMetadataVerify(ctx, "", 5)
		if verr != nil {
			metaCheck = map[string]any{"ok": false, "reason": verr.Error()}
		} else {
			metaCheck = metadataVerifyMap(rep)
		}
	}
	recordAudit(ctx, c, "dr_drill", "backup", ptrx.Deref(req.Name, ""), "灾备演练")
	response.Success(c, map[string]any{
		"mode":           "single",
		"reports":        []any{rep},
		"metadata_check": metaCheck,
	})
}

// metadataVerifyMap core 抽样校验报告 → 响应结构（与历史输出逐键一致：
// backup_dir 未配置时仅 {ok,reason}，否则 {checked,ok,warnings}）。
func metadataVerifyMap(rep corebridge.VerifyReport) map[string]any {
	if rep.Reason != "" {
		return map[string]any{"ok": false, "reason": rep.Reason}
	}
	return map[string]any{
		"checked":  rep.Checked,
		"ok":       rep.OK,
		"warnings": rep.Warnings,
	}
}

// DrillHistory GET /api/v1/ops/dr-drill/history?limit=
func DrillHistory(ctx context.Context, c *app.RequestContext) {
	limit := convert.Str2Int(c.Query("limit"), 0)
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

// ExportDrillHistory GET /api/v1/ops/dr-drill/export?format=json|csv&limit=
func ExportDrillHistory(ctx context.Context, c *app.RequestContext) {
	format := c.Query("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		response.BadRequest(c, "format must be json or csv")
		return
	}
	limit := convert.Str2Int(c.Query("limit"), 0)
	if limit <= 0 {
		limit = 100
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	contentType, data, err := svc.ExportDrillHistory(format, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if format == "csv" {
		c.Header("Content-Disposition", `attachment; filename="dr-drill-report.csv"`)
	} else {
		c.Header("Content-Disposition", `attachment; filename="dr-drill-report.json"`)
	}
	c.Data(consts.StatusOK, contentType, data)
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
		// 冷备没启用不是"内部错误" —— 回 400 并说清怎么开,
		// 别让人对着 500 "internal server error" 猜。
		if errors.Is(err, corebridge.ErrBackupDisabled) {
			response.BadRequest(c, "冷备未启用:配置 sync.backup_dir 后再校验完整性")
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, res)
}

// RPOReport GET /api/v1/ops/rpo?max_seconds=
func RPOReport(ctx context.Context, c *app.RequestContext) {
	maxSec := convert.Str2Int64(c.Query("max_seconds"), 0)
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
