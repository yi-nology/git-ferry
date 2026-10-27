package tools

import (
	"context"
	"encoding/json"
	"github.com/oekit/pkg/errors"

	"github.com/yi-nology/git-ferry/internal/corebridge"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// 历史同步(histsync)工具:两个各自有完整历史的仓库之间按内容差异同步。
//
// 与镜像中心的身份镜像模式并存,语义不同:
//   - 镜像中心:同一仓库的多个身份,每 tag 一个孤儿快照;
//   - 历史同步:两个有完整历史的仓库,按内容差异叠加,目标树既有提交原样保留。
//
// **通道优先**:配置存在通道里(DB 持久化),工具按 channel id 查配置,
// 不让调用方每次重传一整套规则。
//
// 工具面只暴露只读动作 —— apply 会产生本地提交、push 是对外可见动作,
// 二者都留在 CLI 与人工确认路径上。

// historyChannelRef 是「按通道查」类工具的入参(只读)。
//
// **只读工具不要带写参数**:入参 schema 是模型看到的唯一契约 ——
// 给 list/plan/preview 带一个 force_push,模型会以为它有用,
// 实际被静默忽略;而给 list_history_overrides 强制要求 path/verdict,
// 这个只读工具对模型直接不可用(必填参数缺失 → schema 校验失败)。
type historyChannelRef struct {
	ChannelID uint `json:"channel_id" jsonschema:"历史同步通道 ID"`
}

// historyChannelInput 是「会改变对外状态」的工具的入参(仅 sync 用)。
type historyChannelInput struct {
	ChannelID uint `json:"channel_id" jsonschema:"历史同步通道 ID"`
	// ForcePush 强制推送:远端与本次移植的基线已分叉也照样覆盖。
	//
	// 默认不强制 —— 强推会把远端已有的提交**静默丢掉**。
	ForcePush bool `json:"force_push,omitempty" jsonschema:"强制推送(覆盖远端已有提交,危险)"`
}

// listHistoryChannelsInput 是「列出通道」的入参。
//
// **刻意不带 channel_id**:列全部通道不需要指定某个通道 ——
// 复用 historyChannelRef 会让 schema 把 channel_id 标成**必填**
// (json tag 没有 omitempty),而这个工具根本用不上它:
// 模型只能被迫随便填一个 id,或者干脆调不通这个工具。
//
// 顺带把**硬编码的 100 条上限**换成真分页 ——
// 通道多了以后,模型看到的是被静默截断的列表(它并不知道还有下一页)。
type listHistoryChannelsInput struct {
	Page  int `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
	Limit int `json:"limit,omitempty" jsonschema:"每页条数,默认 20,最大 100"`
}

// listHistoryChannels 列出历史同步通道(分页)。
func (r *Registry) listHistoryChannels(ctx context.Context, in listHistoryChannelsInput) (map[string]any, error) {
	page := in.Page
	if page <= 0 {
		page = 1
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	list, total, err := r.svc.ListHistoryChannels(ctx, (page-1)*limit, limit)
	if err != nil {
		return nil, errors.Wrap(err, "列出历史同步通道失败")
	}
	return map[string]any{
		"total": total, "page": page, "limit": limit, "channels": list,
	}, nil
}

// planHistoryChannel 按通道生成同步计划(只读)。
func (r *Registry) planHistoryChannel(ctx context.Context, in historyChannelRef) (map[string]any, error) {
	plan, err := r.svc.HistoryPlan(ctx, in.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "按通道生成历史同步计划失败")
	}
	return map[string]any{
		"channel_id": plan.ChannelID,
		"summary":    plan.Summary,
		"files":      plan.Files,
		"commits":    plan.Commits,
	}, nil
}

// resolveHistoryChannel 按通道生成计划并计算 hold 的自动解法,返回待人工裁决的冲突卷宗。
//
// 这就是「AI 建议器」的输入:卷宗里给出基点/目标侧/源侧三方内容,
// AI 据此给出判定与依据,人工确认后才应用。
func (r *Registry) resolveHistoryChannel(ctx context.Context, in historyChannelRef) (map[string]any, error) {
	plan, err := r.svc.HistoryResolve(ctx, in.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "按通道计算历史同步解法失败")
	}
	out := map[string]any{
		"channel_id":  plan.ChannelID,
		"summary":     plan.Summary,
		"resolutions": plan.Resolutions,
		"dossiers":    plan.Dossiers,
	}
	if len(plan.Dossiers) == 0 {
		out["note"] = "所有 hold 文件均已自动解决,无需人工裁决"
	}
	return out, nil
}

// listHistoryRunsInput 是「列执行记录」的入参。
//
// **刻意带分页**:以前硬编码 50 条上限、且入参只有 channel_id ⇒
// 模型想看更多也拿不到,而输出里的 total 却是**真实总数**
// ⇒ 两个数字对不上,模型无从判断自己看到的是不是全部
// (同一个坑在 list_histsync_channels 上已经修过一遍,这里是漏网的同一个)。
type listHistoryRunsInput struct {
	ChannelID uint `json:"channel_id" jsonschema:"历史同步通道 ID"`
	Page      int  `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
	Limit     int  `json:"limit,omitempty" jsonschema:"每页条数,默认 20,最大 100"`
}

// listHistoryRuns 列出通道的执行记录(审计与回滚锚点)。
//
// 截断必须**说出口**:returned / total 两个数字都在,且被截断时给 truncated +
// notice ⇒ 模型能判断"还有更多,需要翻页",而不是把这一页当成全部。
func (r *Registry) listHistoryRuns(ctx context.Context, in listHistoryRunsInput) (map[string]any, error) {
	page := in.Page
	if page <= 0 {
		page = 1
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	list, total, err := r.svc.ListHistoryRuns(ctx, in.ChannelID, (page-1)*limit, limit)
	if err != nil {
		return nil, errors.Wrap(err, "列出历史同步执行记录失败")
	}
	out := map[string]any{
		"channel_id": in.ChannelID,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"returned":   len(list),
		"runs":       list,
	}
	if int64(len(list)) < total {
		out["truncated"] = true
		out["notice"] = "本页只返回了部分执行记录,还有更多,请用 page 继续翻页"
	}
	return out, nil
}

// verifyHistoryChannelAudit 校验通道内执行记录的哈希链。
func (r *Registry) verifyHistoryChannelAudit(ctx context.Context, in historyChannelRef) (map[string]any, error) {
	res, err := r.svc.VerifyHistoryChannelChain(ctx, in.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "校验历史同步执行记录链失败")
	}
	return map[string]any{
		"channel_id": in.ChannelID,
		"ok":         res.OK,
		"checked":    res.Checked,
		"broken_at":  res.BrokenAt,
		"message":    res.Message,
	}, nil
}

// historyOverrideListInput 是「列裁决」的入参(只读,只要通道 ID)。
type historyOverrideListInput struct {
	ChannelID uint `json:"channel_id" jsonschema:"历史同步通道 ID"`
}

// historyOverrideInput 是裁决写入的入参。
type historyOverrideInput struct {
	ChannelID uint   `json:"channel_id" jsonschema:"历史同步通道 ID"`
	Path      string `json:"path" jsonschema:"文件路径(裁决定位键,目标侧路径)"`
	Verdict   string `json:"verdict" jsonschema:"裁决:keep_target|take_source|merged"`
	Content   string `json:"content,omitempty" jsonschema:"裁决后的内容(仅 merged 用)"`
	Reason    string `json:"reason,omitempty" jsonschema:"裁决理由(审计用)"`
	DecidedBy string `json:"decided_by,omitempty" jsonschema:"裁决者标识(人工或 AI)"`
}

// listHistoryOverrides 列出通道的裁决。
func (r *Registry) listHistoryOverrides(ctx context.Context, in historyOverrideListInput) (map[string]any, error) { //nolint:gocritic // hugeParam: eino utils.InferTool 要求入参是结构体值,指针会破坏入参 schema 推断
	list, err := r.svc.ListHistoryOverrides(ctx, in.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "列出历史同步裁决失败")
	}
	return map[string]any{"channel_id": in.ChannelID, "overrides": list}, nil
}

// setHistoryOverride 写入一条裁决(需要用户确认)。
//
// 裁决会改变实际发布的内容,因此按**危险工具**处理:
// 模型给出判定与依据 → 弹出确认 → 用户确认后才写入。
// 这正是「AI 建议 + 人工确认」的闭环落点。
func (r *Registry) setHistoryOverride(ctx context.Context, in historyOverrideInput) (string, error) { //nolint:gocritic // hugeParam: 同上——工具入参形状由 eino 的 schema 推断决定
	return r.dangerGuard(ctx, "set_histsync_override", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in historyOverrideInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		out, err := r.svc.SetHistoryOverride(ctx, in.ChannelID, corebridge.HistoryOverrideInput{
			Path:       in.Path,
			SourcePath: "",
			Verdict:    in.Verdict,
			Content:    in.Content,
			Reason:     in.Reason,
			DecidedBy:  in.DecidedBy,
		})
		if err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": err.Error()}), nil
		}
		return marshalJSON(map[string]any{
			"status": "ok", "message": "裁决已写入;重新 resolve 后按裁决出解",
			"override": out,
		}), nil
	})
}

// previewHistoryChannel 预检:摊开这次同步的全部影响面(不推送)。
//
// 报告含:计划统计、将产生的提交、将改动的文件、**门禁结果(推出去能不能编译)**、
// 每个推送目标的 ref 变化与是否需要强推、待人工裁决的冲突。
func (r *Registry) previewHistoryChannel(ctx context.Context, in historyChannelRef) (map[string]any, error) {
	rep, err := r.svc.HistoryPreview(ctx, in.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "历史同步预检失败")
	}
	return map[string]any{
		"channel_id":        rep.ChannelID,
		"channel_name":      rep.ChannelName,
		"base_head":         rep.BaseHead,
		"new_head":          rep.NewHead,
		"summary":           rep.Summary,
		"gate":              rep.Gate,
		"targets":           rep.Targets,
		"needs_force":       rep.NeedsForce,
		"warnings":          rep.Warnings,
		"applied_files":     rep.AppliedFiles,
		"skipped_files":     rep.SkippedFiles,
		"protected_count":   rep.ProtectedCount,
		"pending_conflicts": rep.PendingConflicts,
	}, nil
}

// syncHistoryChannel 一次跑完「预检 → 移植 → 推送」。
//
// **危险操作**:会真的推送到远端(先留底后发布)。
// 服务端在有阻塞项(门禁不过 / 有待裁决冲突 / 需强推)时会停下并报告,
// 不擅自往下走。
func (r *Registry) syncHistoryChannel(ctx context.Context, in historyChannelInput) (string, error) {
	// **强制推送要 admin**:它会覆盖远端已有的提交。
	// 之前这条只在 HTTP handler 判过,MCP / AI 助手两条路没判 ——
	// operator 就能强推覆盖掉远端别人的提交。
	//
	// 判在**登记确认之前**:没权限就别让用户走完确认流程才发现。
	role := authz.RoleFromContext(ctx)
	if in.ForcePush && !authz.IsAdmin(role) {
		return marshalJSON(map[string]any{
			"status": "failed",
			"error":  "forbidden: 强制推送需要 admin 角色(当前:" + string(role) + ")",
		}), nil
	}
	return r.dangerGuard(ctx, "sync_histsync_channel", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in historyChannelInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		// 再判一次(防止将来有人绕过外层的检查)。
		role := authz.RoleFromContext(ctx)
		if in.ForcePush && !authz.IsAdmin(role) {
			return marshalJSON(map[string]any{
				"status": "failed",
				"error":  "forbidden: 强制推送需要 admin 角色(当前:" + string(role) + ")",
			}), nil
		}
		rep, err := r.svc.HistorySync(ctx, corebridge.HistorySyncOptions{
			ChannelID: in.ChannelID, ForcePush: in.ForcePush,
			// core 只认「有人批准过」这个事实。
			ForcePushApprovedBy: "role:" + string(role),
		})
		if err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": err.Error()}), nil
		}
		if len(rep.Blocked) > 0 {
			var summary any
			if rep.Preview != nil {
				summary = rep.Preview.Summary
			}
			return marshalJSON(map[string]any{
				"status": "blocked", "message": "预检发现阻塞项,未执行移植与推送",
				"blocked": rep.Blocked,
				"summary": summary,
			}), nil
		}
		// 不假设 Push / Preview 非空:报告是可选的,崩在这里等于整个工具不可用。
		var heads map[string]string
		var pushed []string
		if rep.Push != nil {
			heads = rep.Push.Heads
			pushed = rep.Push.Pushed
		}
		var summary any
		if rep.Preview != nil {
			summary = rep.Preview.Summary
		}
		return marshalJSON(map[string]any{
			"status": "ok", "message": "同步完成",
			"head": heads, "pushed": pushed,
			"summary": summary,
		}), nil
	})
}

// historyCredentialInput 是凭据轮换的入参。
type historyCredentialInput struct {
	TargetID   uint   `json:"target_id" jsonschema:"推送目标 ID"`
	CredType   string `json:"cred_type" jsonschema:"凭据类型:none|token|ssh_key"`
	Credential string `json:"credential,omitempty" jsonschema:"凭据明文(只在传输中出现,落库即加密)"`
	Username   string `json:"username,omitempty" jsonschema:"HTTPS Basic 用户名(token 类用)"`
}

// rotateHistoryTargetCredential 轮换推送目标的凭据(危险操作)。
func (r *Registry) rotateHistoryTargetCredential(ctx context.Context, in historyCredentialInput) (string, error) {
	return r.dangerGuard(ctx, "rotate_history_target_credential", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in historyCredentialInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		out, err := r.svc.RotateHistoryTargetCredential(ctx, in.TargetID, corebridge.HistoryCredentialInput{
			CredType: in.CredType, Credential: in.Credential, Username: in.Username,
		})
		if err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": err.Error()}), nil
		}
		return marshalJSON(map[string]any{
			"status": "ok", "message": "凭据已轮换",
			"target": out.Name, "cred_type": out.CredType,
		}), nil
	})
}
