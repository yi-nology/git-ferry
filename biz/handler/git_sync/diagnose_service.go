package git_sync

import (
	"context"
	"fmt"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// DiagnoseRun GET /api/v1/ops/diagnose?run_id=
// 对失败 run 做结构化诊断:错误分类 → 可能原因 → 建议动作。
// 借鉴 AI 助手的解释能力,但无需模型即可给出规则化结论。
func DiagnoseRun(ctx context.Context, c *app.RequestContext) {
	var req ops.DiagnoseReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if uint(req.RunId) == 0 {
		response.BadRequest(c, "run_id is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	run, taskKey, err := findRunByID(ctx, svc, uint(req.RunId))
	if err != nil {
		response.NotFound(c, "run not found")
		return
	}

	d := diagnoseRun(run)
	d["run_id"] = run.ID
	d["task_key"] = taskKey
	d["status"] = run.Status
	d["error_message"] = run.ErrorMessage
	d["error_type"] = run.ErrorType
	d["retry_total"] = run.RetryTotal

	// 附最近 3 条历史,便于看是否反复失败
	if hist, _, herr := svc.ListHistory(ctx, taskKey, 0, 5); herr == nil {
		timeline := []map[string]any{}
		for _, h := range hist {
			end := ""
			if h.EndTime != nil {
				end = h.EndTime.Format("2006-01-02 15:04:05")
			}
			timeline = append(timeline, map[string]any{
				"run_id": h.ID, "status": h.Status, "end_at": end,
				"error_type": h.ErrorType,
			})
		}
		d["recent_runs"] = timeline
	}
	response.Success(c, d)
}

// diagnoseRun 规则化诊断(错误分类 + 关键词)。
func diagnoseRun(run *corebridge.SyncRun) map[string]any {
	et := run.ErrorType
	msg := strings.ToLower(run.ErrorMessage + " " + run.Details)

	var causes []string
	var actions []string
	severity := "warning"

	switch et {
	case "auth":
		severity = "critical"
		causes = append(causes, "访问令牌无效或权限不足")
		actions = append(actions,
			"检查平台 access_token 是否过期",
			"确认 token 有 repo/push 权限",
			"若用 SSH,检查密钥与部署权限")
	case "network":
		causes = append(causes, "网络不可达或超时")
		actions = append(actions,
			"检查目标/源平台连通性",
			"配置 proxy 或改内网镜像地址",
			"调大 sync.default_timeout 后重试")
	case "divergent", "conflict":
		severity = "critical"
		causes = append(causes, "目标分支与源分支已分叉,拒绝覆盖")
		actions = append(actions,
			"对比目标独有提交,确认可丢弃后关闭 keep_divergent",
			"或先把目标变更合回源,再重试")
	case "config":
		causes = append(causes, "仓库/分支/平台配置不正确")
		actions = append(actions,
			"核对 source/target repo key 与分支名",
			"确认仓库在平台上仍存在")
	default:
		if strings.Contains(msg, "lfs") {
			causes = append(causes, "LFS 对象同步失败")
			actions = append(actions, "确认运行环境安装 git-lfs,或关闭 git_lfs")
		}
		if strings.Contains(msg, "bundle") {
			causes = append(causes, "冷备 bundle 生成失败")
			actions = append(actions, "检查 sync.backup_dir 可写,磁盘空间充足")
		}
		if strings.Contains(msg, "wiki") {
			causes = append(causes, "wiki 同步失败(不影响代码)")
			actions = append(actions, "确认平台已启用 wiki,或关闭 sync_wiki")
		}
		if strings.Contains(msg, "push") {
			causes = append(causes, "推送被拒绝")
			actions = append(actions, "检查目标分支保护规则;必要时开 git_force(谨慎)")
		}
		if len(causes) == 0 {
			causes = append(causes, "未归类错误")
			actions = append(actions, "查看 details 步骤链定位失败环节", "必要时 POST /ops/rebuild 全量重建")
		}
	}

	if run.RetryTotal > 0 {
		actions = append(actions, fmt.Sprintf("已自动重试 %d 次仍失败,建议人工介入", run.RetryTotal))
	}

	return map[string]any{
		"severity":     severity,
		"error_type":   et,
		"likely_cause": causes,
		"suggestions":  actions,
	}
}
