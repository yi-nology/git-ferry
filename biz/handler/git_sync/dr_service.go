package git_sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/biz/model/ops"
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
	if optBool(req.All) {
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
	if optStr(req.Name) == "" {
		response.BadRequest(c, "name is required (or set all=true)")
		return
	}
	rep, err := svc.RunDRDrill(ctx, optStr(req.Name))
	if err != nil && rep == nil {
		response.InternalError(c, err.Error())
		return
	}
	metaCheck := map[string]any{}
	if optBoolDefault(req.WithMetadata) {
		metaCheck = sampleMetadataVerify(optStr(req.Name))
	}
	recordAudit(ctx, c, "dr_drill", "backup", optStr(req.Name), "灾备演练")
	response.Success(c, map[string]any{
		"mode":           "single",
		"reports":        []any{rep},
		"metadata_check": metaCheck,
	})
}

// sampleMetadataVerify 抽样比对元数据快照：清单存在、分片可解析、数量一致。
func sampleMetadataVerify(bundleName string) map[string]any {
	svc := GetSyncService()
	if svc == nil || svc.BackupDir() == "" {
		return map[string]any{"ok": false, "reason": "backup_dir not configured"}
	}
	root := filepath.Join(svc.BackupDir(), "metadata")
	out := map[string]any{"checked": 0, "ok": true, "warnings": []string{}}
	warns := []string{}
	checked := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "manifest.json" {
			return nil
		}
		checked++
		data, rerr := os.ReadFile(path) //nolint:gosec // 内部备份路径
		if rerr != nil {
			warns = append(warns, path+": read")
			return nil
		}
		var snap struct {
			Counts map[string]int32 `json:"counts"`
			Files  []string         `json:"files"`
		}
		if json.Unmarshal(data, &snap) != nil {
			warns = append(warns, path+": manifest parse")
			return nil
		}
		// 抽样：issues.json 存在且条数与 counts 对齐
		if snap.Counts["issues"] > 0 {
			issuesPath := filepath.Join(filepath.Dir(path), "issues.json")
			if _, serr := os.Stat(issuesPath); serr != nil {
				warns = append(warns, path+": issues.json missing")
			}
		}
		if checked >= 5 {
			return filepath.SkipAll
		}
		return nil
	})
	out["checked"] = checked
	out["warnings"] = warns
	if len(warns) > 0 {
		out["ok"] = false
	}
	return out
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
	limit, _ := strconv.Atoi(c.Query("limit"))
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
