package git_sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
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
		response.FromError(c, err)
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
		response.FromError(c, err)
		return
	}
	recordAuditDetail(ctx, c, "history.channel.create", "history-channel",
		strconv.FormatUint(uint64(out.ID), 10), "创建历史同步通道 "+out.Name,
		jsonDetail(map[string]any{
			"module_source": req.ModuleSource, "module_target": req.ModuleTarget,
			// **本机路径不落审计明细**。
			//
			// 审计明细进**不可篡改的哈希链**(见 model.OperationLog 的 payload),
			// 写进去就等于永久提交:改了会让整条链报「被篡改」,
			// 而它本身只是「在哪台机器的哪个目录跑的」——
			// 通道表里本来就有这两个路径(可对照),不必在链里再存一份绝对路径。
			//
			// 留「目录名 + 短哈希」:既能对上通道配置,又不泄露执行机的目录结构。
			"source_dir": localDirRef(req.SourceDir), "target_dir": localDirRef(req.TargetDir),
		}))
	response.Success(c, out)
}

// localDirRef 把本机路径降级成「目录名 + 短哈希」。
//
// **为什么不给绝对路径**:审计明细进**不可篡改的哈希链**
// (见 model.OperationLog 的 payload)—— 写进去就等于永久提交:
// 事后想改会让整条链报「被篡改」。
// 而它本身只是「在哪台机器的哪个目录跑的」——
// 通道表里本来就存着这两个路径(可对照),不必在链里再存一份;
// 绝对路径还会把执行机的目录结构与操作者用户名永久写进审计记录。
//
// 留「目录名 + 短哈希」:同一个路径 → 同一个 ref(可对照、可去重),
// 又不泄露本机布局。
func localDirRef(p *string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return ""
	}
	abs, err := filepath.Abs(*p)
	if err != nil {
		abs = *p
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Base(abs) + "#" + hex.EncodeToString(sum[:])[:12]
}

// HistoryChannelGet 通道详情(含推送目标)。
func HistoryChannelGet(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	out, err := GetSyncService().GetHistoryChannel(ctx, id)
	if err != nil {
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
		return
	}
	recordAuditDetail(ctx, c, "history.target.create", "history-target",
		strconv.FormatUint(uint64(out.ID), 10), "创建推送目标 "+out.Name,
		jsonDetail(map[string]any{
			"channel_id": id, "repo_url": redactRepoURL(req.RepoURL),
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
		response.FromError(c, err)
		return
	}
	// 改推送目标 = 改「内容发到哪里」,必须留痕。
	// URL 要脱敏:它可能内嵌 token(见 redactRepoURL)。
	recordAuditDetail(ctx, c, "history.target.update", "history-target",
		strconv.FormatUint(uint64(id), 10), "更新推送目标",
		jsonDetail(map[string]any{
			"name":           out.Name,
			"url":            redactRepoURL(out.RepoURL),
			"has_credential": out.HasCredential,
		}))
	response.Success(c, out)
}

// HistoryTargetDelete 删除推送目标。
func HistoryTargetDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncID(c, "id")
	if !ok {
		return
	}
	if err := GetSyncService().DeleteHistoryTarget(ctx, id); err != nil {
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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

// historyPushReq 是推送的请求体(可选)。
type historyPushReq struct {
	// ForcePush 强制推送:远端与本次移植的基线已分叉也照样覆盖。
	//
	// 默认不强制 —— 强推会把远端已有的提交静默丢掉,
	// 必须由人明确点头才覆盖。
	//
	// **字段名与 /sync 端点统一**。原名是 `force`,而同一套 API 里的
	// `/sync` 端点用 `force` 表示「忽略阻塞项继续」——
	// 同一个键在两个兄弟端点上语义相反,CLI 被迫写一层手工转译
	// (`--force-push` 传过去叫 `force`),传错键还会被**静默丢弃**。
	// 现在两边都用 `force_push`,语义唯一。
	ForcePush bool `json:"force_push"`
	// Force 是**已废弃的旧字段名**(语义与 force_push 相同:强制推送)。
	//
	// 保留它只是为了**明确报错**而不是静默忽略:直接删掉的话,
	// 旧调用方的 `{"force":true}` 会被服务端当成「没传」——
	// 用户以为强推了,实际没推(比报错更危险)。
	Force *bool `json:"force"`
}

// HistoryChannelPush 按通道推送最近一次执行记录的产物(危险)。
func HistoryChannelPush(ctx context.Context, c *app.RequestContext) {
	id, ok := histsyncChannelID(c)
	if !ok {
		return
	}
	// 请求体可选:{"force_push": true} 表示强制推送(覆盖远端已有的提交)。
	var req historyPushReq
	if len(c.Request.Body()) > 0 {
		if err := c.BindAndValidate(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	// **旧字段名明确报错**:同一个键 `force` 在 `/sync` 端点表示
	// 「忽略阻塞项继续」,在 `/push` 却表示「强制推送」——
	// 语义相反的同名键迟早被用错。现在 `/push` 只认 `force_push`,
	// 旧调用方收到明确的指路,而不是「参数被静默忽略」。
	if req.Force != nil {
		response.BadRequest(c, "force 已改名为 force_push(语义不变:强制推送,覆盖远端已有提交);"+
			"注意 /sync 的 force 是「忽略阻塞项继续」,两者语义不同")
		return
	}
	// **强制推送要 admin**。
	//
	// 强推会覆盖远端已有的提交 —— 平台在 ops 侧对同类破坏性动作
	// 做了「申请 + admin 批准」两段式,这里不该只要 operator。
	if req.ForcePush {
		if role := GetAuthRole(c); role != RoleAdmin {
			response.Error(c, 403, "forbidden: 强制推送需要 admin 角色(当前:"+string(role)+")")
			return
		}
	}
	out, err := GetSyncService().HistoryPushWithOptions(ctx, id, corebridge.HistoryPushOptions{
		Force: req.ForcePush,
		// 批准者:角色已判定为 admin,这里把操作人记下来(core 只认「有人批准过」)。
		ForceApprovedBy: approverID(c),
	})
	if err != nil {
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
		response.FromError(c, err)
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
	// ForcePush 强制推送:远端与本次移植的基线已分叉也照样覆盖。
	//
	// 与 Force 分开:「忽略阻塞项继续」和「覆盖远端已有的提交」是两件事,
	// 后者会丢掉远端别人的提交,必须显式指定。
	ForcePush bool `json:"force_push"`
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
	// 强制推送要 admin(与单条 push 一致)。
	if req.ForcePush {
		if role := GetAuthRole(c); role != RoleAdmin {
			response.Error(c, 403, "forbidden: 强制推送需要 admin 角色(当前:"+string(role)+")")
			return
		}
	}
	out, err := GetSyncService().HistorySync(ctx, corebridge.HistorySyncOptions{
		ChannelID: id, Force: req.Force, ForcePush: req.ForcePush,
		// 批准者:角色已判定为 admin,这里把操作人记下来(core 只认「有人批准过」)。
		ForcePushApprovedBy: approverID(c),
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	if len(out.Blocked) > 0 {
		for _, b := range out.Blocked {
			recordAudit(ctx, c, "history.sync.blocked", "history-channel",
				strconv.FormatUint(uint64(id), 10), "历史同步被阻塞:"+b)
		}
	} else {
		// 成功路径也要留痕:一次 sync 会真的推送到远端。
		detail := map[string]any{}
		if out.Apply != nil {
			detail["apply_run_id"] = out.Apply.ID
			detail["head"] = out.Apply.Head
		}
		if out.Push != nil {
			detail["push_run_id"] = out.Push.RunID
			detail["pushed"] = out.Push.Pushed
		}
		recordAuditDetail(ctx, c, "history.sync", "history-channel",
			strconv.FormatUint(uint64(id), 10), "历史同步:预检→移植→推送", jsonDetail(detail))
	}
	response.Success(c, out)
}

// redactRepoURL 脱敏仓库 URL,用于审计日志。
//
// 推送目标的 URL **可能内嵌凭据**:
//   - userinfo 段(https://user:token@host/...)
//   - 常见的 token 类查询参数(?access_token=...)
//
// 这些 URL 会进审计日志(落库、可查询),原样记等于把凭据抄一份。
func redactRepoURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		// 解析不了就退到「只留主机名」,别把原文写进日志。
		return "<unparsable-url>"
	}
	if u.User != nil {
		u.User = url.User("***")
	}
	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			switch strings.ToLower(key) {
			case "token", "access_token", "private_token", "password", "passwd", "secret", "api_key":
				q.Set(key, "***")
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// approverID 取当前操作人标识(用于记录「谁批准了强制推送」)。
func approverID(c *app.RequestContext) string {
	if u := GetAuthUser(c); u != "" {
		return u
	}
	return "unknown"
}
