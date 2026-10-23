package git_sync

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 历史同步(histsync) =====
//
// 两个各自有完整历史的仓库之间按内容差异同步。
// 与镜像中心的身份镜像模式并存:
//   - /mirror:同一仓库的多个身份,每 tag 一个孤儿快照;
//   - /histsync:两个有完整历史的仓库,按内容差异叠加,目标树既有提交原样保留。
//
// **接口是通道优先的**:配置在通道里,执行按 channel id 进行,
// 调用方不重传规则、也不回传服务端的临时路径。

// histsyncChannelID 取路径上的通道 ID。
func histsyncChannelID(c *app.RequestContext) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}

// histsyncID 取路径上的任意正整数 ID。
func histsyncID(c *app.RequestContext, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid "+name)
		return 0, false
	}
	return uint(id), true
}

// ---------- 通道 ----------

// HistoryChannelList 列出历史同步通道。
func HistoryChannelList(ctx context.Context, c *app.RequestContext) {
	page := parsePage(c)
	list, total, err := GetSyncService().ListHistoryChannels(ctx, page.Offset, page.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"list": list, "total": total})
}

// HistoryChannelCreate 新建历史同步通道。
func HistoryChannelCreate(ctx context.Context, c *app.RequestContext) {
	var req corebridge.HistoryChannelInput
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().CreateHistoryChannel(ctx, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.channel.create", "history-channel",
		strconv.FormatUint(uint64(out.ID), 10), "创建历史同步通道 "+out.Name,
		jsonDetail(map[string]any{
			"module_source": req.ModuleSource, "module_target": req.ModuleTarget,
			"source_dir": req.SourceDir, "target_dir": req.TargetDir,
		}))
	response.Success(c, out)
}

// HistoryChannelGet 通道详情(含推送目标)。
func HistoryChannelGet(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().GetHistoryChannel(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// HistoryChannelUpdate 更新通道。
func HistoryChannelUpdate(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	var req corebridge.HistoryChannelInput
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().UpdateHistoryChannel(ctx, id, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.channel.update", "history-channel",
		strconv.FormatUint(uint64(id), 10), "更新历史同步通道 "+out.Name, "")
	response.Success(c, out)
}

// HistoryChannelDelete 删除通道(连同其推送目标)。
func HistoryChannelDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	if err := GetSyncService().DeleteHistoryChannel(ctx, id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "history.channel.delete", "history-channel",
		strconv.FormatUint(uint64(id), 10), "删除历史同步通道")
	response.Success(c, map[string]any{"deleted": id})
}

// ---------- 推送目标 ----------

// HistoryTargetList 列出通道的推送目标(顺序 = 推送优先级)。
func HistoryTargetList(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	list, err := GetSyncService().ListHistoryTargets(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"list": list})
}

// HistoryTargetCreate 新建推送目标(凭据加密落库,响应不回显)。
func HistoryTargetCreate(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	var req corebridge.HistoryTargetInput
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().CreateHistoryTarget(ctx, id, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.target.create", "history-target",
		strconv.FormatUint(uint64(out.ID), 10), "创建推送目标 "+out.Name,
		jsonDetail(map[string]any{
			"channel_id": id, "repo_url": req.RepoURL,
			"cred_type": req.CredType, "has_credential": out.HasCredential,
		}))
	response.Success(c, out)
}

// HistoryTargetUpdate 更新推送目标。
func HistoryTargetUpdate(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	var req corebridge.HistoryTargetInput
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().UpdateHistoryTarget(ctx, id, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// HistoryTargetDelete 删除推送目标。
func HistoryTargetDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	if err := GetSyncService().DeleteHistoryTarget(ctx, id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "history.target.delete", "history-target",
		strconv.FormatUint(uint64(id), 10), "删除推送目标")
	response.Success(c, map[string]any{"deleted": id})
}

// ---------- 执行(通道优先,无请求体) ----------

// HistoryChannelPlan 按通道生成同步计划(纯读)。
func HistoryChannelPlan(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryPlan(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// HistoryChannelResolve 按通道生成计划并计算 hold 的自动解法(含冲突卷宗)。
func HistoryChannelResolve(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryResolve(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// HistoryChannelApply 按通道把计划落到目标树,产出执行记录(本地,不推送)。
func HistoryChannelApply(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryApply(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.apply", "history-run",
		strconv.FormatUint(uint64(out.ID), 10), "历史同步:内容移植",
		jsonDetail(map[string]any{
			"channel_id": id, "status": out.Status,
			"base_head": out.BaseHead, "head": out.Head,
			"entry_hash": out.EntryHash, "prev_hash": out.PrevHash,
		}))
	response.Success(c, out)
}

// HistoryChannelPush 按通道推送最近一次执行记录的产物(危险)。
func HistoryChannelPush(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryPush(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.push", "history-run",
		strconv.FormatUint(uint64(out.RunID), 10), "历史同步:推送",
		jsonDetail(map[string]any{
			"channel_id": id, "pushed": out.Pushed, "heads": out.Heads,
		}))
	response.Success(c, out)
}

// ---------- 执行记录与回滚 ----------

// HistoryRunList 列出通道的执行记录。
func HistoryRunList(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	page := parsePage(c)
	list, total, err := GetSyncService().ListHistoryRuns(ctx, id, page.Offset, page.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"list": list, "total": total})
}

// HistoryRunGet 执行记录详情(审计材料:计划统计/commit 映射/检测报告/ref 对照)。
func HistoryRunGet(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	out, err := GetSyncService().GetHistoryRun(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// historyRollbackReq 是回滚请求体。
type historyRollbackReq struct {
	// Ref 回滚到的 ref(通常是某次同步前的 HEAD)。
	Ref string `json:"ref"`
}

// HistoryRunRollback 把目标树回滚到指定 ref(危险)。
func HistoryRunRollback(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	var req historyRollbackReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().HistoryRollback(ctx, id, req.Ref)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.rollback", "history-run",
		strconv.FormatUint(uint64(id), 10), "历史同步:回滚到 "+req.Ref,
		jsonDetail(map[string]any{"ref": req.Ref, "pushed": out.Pushed, "heads": out.Heads}))
	response.Success(c, out)
}

// HistoryChannelAudit 校验通道内执行记录的哈希链。
func HistoryChannelAudit(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().VerifyHistoryChannelChain(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// jsonDetail 把明细序列化为审计 Detail。
func jsonDetail(v map[string]any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

// ---------- 裁决覆盖(人工/AI 拍板) ----------

// HistoryOverrideList 列出通道的裁决。
func HistoryOverrideList(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	list, err := GetSyncService().ListHistoryOverrides(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"list": list})
}

// HistoryOverrideSet 写入(或覆盖)一条裁决。
func HistoryOverrideSet(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	var req corebridge.HistoryOverrideInput
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().SetHistoryOverride(ctx, id, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.override", "history-override",
		strconv.FormatUint(uint64(out.ID), 10), "历史同步:裁决 "+out.Path,
		jsonDetail(map[string]any{
			"channel_id": id, "path": out.Path, "verdict": out.Verdict,
			"decided_by": out.DecidedBy, "reason": out.Reason,
		}))
	response.Success(c, out)
}

// HistoryOverrideDelete 删除一条裁决(回到自动判定)。
func HistoryOverrideDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	if err := GetSyncService().DeleteHistoryOverride(ctx, id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "history.override.delete", "history-override",
		strconv.FormatUint(uint64(id), 10), "历史同步:撤销裁决")
	response.Success(c, map[string]any{"deleted": id})
}

// HistoryChannelPreview 执行预检:把这次同步的全部影响面摊开,但不推送。
//
// 纯读:不推送、不改原始目标仓。物化与门禁都在临时工作副本上进行,
// 与真正执行走的是同一套代码路径 —— 预检结论必须与执行结论一致。
func HistoryChannelPreview(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryPreview(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, out)
}

// HistoryChannelPreviewMarkdown 预检报告的 Markdown 视图(给人读)。
func HistoryChannelPreviewMarkdown(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().HistoryPreviewMarkdown(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(200, out)
}

// ---------- 凭据轮换 ----------

// historyCredentialReq 是凭据轮换请求体。
type historyCredentialReq struct {
	// CredType 凭据类型:none / token / ssh_key。
	CredType string `json:"cred_type"`
	// Credential 凭据明文。
	Credential string `json:"credential"`
	// Username HTTPS Basic 用户名。
	Username string `json:"username"`
	// ExpiresAt 凭据到期时间(RFC3339,可空)。
	ExpiresAt string `json:"expires_at,omitempty"`
}

// HistoryTargetRotate 轮换推送目标的凭据(只换密钥,不动 URL)。
func HistoryTargetRotate(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	var req historyCredentialReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	in := corebridge.HistoryCredentialInput{
		CredType: req.CredType, Credential: req.Credential, Username: req.Username,
	}
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			response.BadRequest(c, "expires_at 必须是 RFC3339 时间")
			return
		}
		in.ExpiresAt = &t
	}
	out, err := GetSyncService().RotateHistoryTargetCredential(ctx, id, in)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditDetail(ctx, c, "history.target.rotate", "history-target",
		strconv.FormatUint(uint64(id), 10), "轮换推送目标凭据 "+out.Name,
		jsonDetail(map[string]any{
			"target_id": id, "cred_type": out.CredType, "decided_by": GetAuthUser(c),
		}))
	response.Success(c, out)
}

// ---------- 一次命令 ----------

// historySyncReq 是一次同步的请求体。
type historySyncReq struct {
	// Force 有阻塞项时也继续。
	Force bool `json:"force"`
}

// HistorySync 一次跑完「预检 → 移植 → 推送」。
//
// 有阻塞项(门禁不过 / 有待裁决冲突 / 需强推)时**停下并报告**,不往下走。
func HistorySync(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	var req historySyncReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := GetSyncService().HistorySync(ctx, corebridge.HistorySyncOptions{
		ChannelID: id, Force: req.Force,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if len(out.Blocked) > 0 {
		for _, b := range out.Blocked {
			recordAudit(ctx, c, "history.sync.blocked", "history-channel",
				strconv.FormatUint(uint64(id), 10), "历史同步被阻塞:"+b)
		}
	}
	response.Success(c, out)
}
