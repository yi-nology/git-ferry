package tools

import (
	"context"
	"encoding/json"
)

// ===== P0-P5 运维工具:灾备 / RPO / 漂移 / 审计链 =====

type rpoInput struct {
	MaxSeconds int64 `json:"max_seconds,omitempty" jsonschema:"RPO 阈值秒数,超过则标记 violated;0=不校验"`
}

func (r *Registry) getRPOReport(_ context.Context, in rpoInput) (string, error) {
	rep, err := r.svc.RPOReport(in.MaxSeconds)
	if err != nil {
		return errJSON("查询 RPO 失败", err), nil
	}
	return marshalJSON(rep), nil
}

type integrityInput struct {
	_ struct{} `json:""`
}

func (r *Registry) getBackupIntegrity(_ context.Context, _ integrityInput) (string, error) {
	res, err := r.svc.VerifyBackupManifest()
	if err != nil {
		return errJSON("校验备份完整性失败", err), nil
	}
	return marshalJSON(res), nil
}

type driftInput struct {
	TaskKeys []string `json:"task_keys,omitempty" jsonschema:"指定任务 key 列表;空=扫描全部启用任务"`
}

func (r *Registry) getDriftReport(ctx context.Context, in driftInput) (string, error) {
	rep, err := r.svc.DetectDrift(ctx, in.TaskKeys)
	if err != nil {
		return errJSON("漂移检测失败", err), nil
	}
	return marshalJSON(rep), nil
}

type auditChainInput struct {
	_ struct{} `json:""`
}

func (r *Registry) getAuditChain(_ context.Context, _ auditChainInput) (string, error) {
	res, err := r.svc.VerifyAuditChain()
	if err != nil {
		return errJSON("审计链校验失败", err), nil
	}
	return marshalJSON(res), nil
}

type drillInput struct {
	Name string `json:"name,omitempty" jsonschema:"要演练的 bundle 文件名;与 all 二选一"`
	All  bool   `json:"all,omitempty" jsonschema:"true=批量演练最近若干份 bundle"`
	Max  int    `json:"max,omitempty" jsonschema:"批量模式最多演练几份,默认 3"`
}

// runDRDrill 危险工具:会真实执行恢复演练(写临时目录),需用户确认。
func (r *Registry) runDRDrill(ctx context.Context, in drillInput) (string, error) {
	return r.dangerGuard(ctx, "run_dr_drill", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in drillInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		if in.All {
			max := in.Max
			if max <= 0 {
				max = 3
			}
			reports, summary, err := r.svc.BatchDRDrill(ctx, nil, max)
			if err != nil {
				return errJSON("批量灾备演练失败", err), nil
			}
			return marshalJSON(map[string]any{
				"status":  "ok",
				"mode":    "batch",
				"summary": summary,
				"reports": reports,
			}), nil
		}
		if in.Name == "" {
			return marshalJSON(map[string]any{"status": "failed", "error": "name 或 all 必须指定其一"}), nil
		}
		rep, err := r.svc.RunDRDrill(ctx, in.Name)
		if err != nil && rep == nil {
			return errJSON("灾备演练失败", err), nil
		}
		return marshalJSON(map[string]any{"status": "ok", "mode": "single", "reports": []any{rep}}), nil
	})
}
