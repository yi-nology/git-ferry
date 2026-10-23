package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yi-nology/git-ferry/internal/corebridge"
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

// historyChannelInput 是按通道操作的入参。
type historyChannelInput struct {
	ChannelID uint `json:"channel_id" jsonschema:"历史同步通道 ID"`
}

// listHistoryChannels 列出历史同步通道。
func (r *Registry) listHistoryChannels(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	list, total, err := r.svc.ListHistoryChannels(ctx, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("列出历史同步通道失败: %w", err)
	}
	return map[string]any{"total": total, "channels": list}, nil
}

// planHistoryChannel 按通道生成同步计划(只读)。
func (r *Registry) planHistoryChannel(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	plan, err := r.svc.HistoryPlan(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("按通道生成历史同步计划失败: %w", err)
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
func (r *Registry) resolveHistoryChannel(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	plan, err := r.svc.HistoryResolve(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("按通道计算历史同步解法失败: %w", err)
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

// listHistoryRuns 列出通道的执行记录(审计与回滚锚点)。
func (r *Registry) listHistoryRuns(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	list, total, err := r.svc.ListHistoryRuns(ctx, in.ChannelID, 0, 50)
	if err != nil {
		return nil, fmt.Errorf("列出历史同步执行记录失败: %w", err)
	}
	return map[string]any{"total": total, "runs": list}, nil
}

// verifyHistoryChannelAudit 校验通道内执行记录的哈希链。
func (r *Registry) verifyHistoryChannelAudit(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	res, err := r.svc.VerifyHistoryChannelChain(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("校验历史同步执行记录链失败: %w", err)
	}
	return map[string]any{
		"channel_id": in.ChannelID,
		"ok":         res.OK,
		"checked":    res.Checked,
		"broken_at":  res.BrokenAt,
		"message":    res.Message,
	}, nil
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
func (r *Registry) listHistoryOverrides(ctx context.Context, in historyOverrideInput) (map[string]any, error) { //nolint:gocritic // hugeParam: eino utils.InferTool 要求入参是结构体值,指针会破坏入参 schema 推断
	list, err := r.svc.ListHistoryOverrides(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("列出历史同步裁决失败: %w", err)
	}
	return map[string]any{"channel_id": in.ChannelID, "overrides": list}, nil
}

// setHistoryOverride 写入一条裁决(需要用户确认)。
//
// 裁决会改变实际发布的内容,因此按**危险工具**处理:
// 模型给出判定与依据 → 弹出确认 → 用户确认后才写入。
// 这正是「AI 建议 + 人工确认」的闭环落点。
func (r *Registry) setHistoryOverride(ctx context.Context, in historyOverrideInput) (string, error) { //nolint:gocritic // hugeParam: 同上——工具入参形状由 eino 的 schema 推断决定
	return r.dangerGuard(ctx, "set_history_override", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
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
func (r *Registry) previewHistoryChannel(ctx context.Context, in historyChannelInput) (map[string]any, error) {
	rep, err := r.svc.HistoryPreview(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("历史同步预检失败: %w", err)
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
	return r.dangerGuard(ctx, "sync_history_channel", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in historyChannelInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		rep, err := r.svc.HistorySync(ctx, corebridge.HistorySyncOptions{ChannelID: in.ChannelID})
		if err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": err.Error()}), nil
		}
		if len(rep.Blocked) > 0 {
			return marshalJSON(map[string]any{
				"status": "blocked", "message": "预检发现阻塞项,未执行移植与推送",
				"blocked": rep.Blocked,
				"summary": rep.Preview.Summary,
			}), nil
		}
		return marshalJSON(map[string]any{
			"status": "ok", "message": "同步完成",
			"head": rep.Push.Heads, "pushed": rep.Push.Pushed,
			"summary": rep.Preview.Summary,
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
