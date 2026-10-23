package corebridge

import (
	"github.com/yi-nology/git-ferry-core/mirror/history"
	coreservice "github.com/yi-nology/git-ferry-core/service"
)

// ===== 历史同步(histsync) =====
//
// 两个各自有完整历史的仓库之间按**内容差异**同步,
// 目标树的既有提交原样保留。
// 与镜像中心(mirror)的身份镜像模式并存,语义不同。
//
// **接口是通道优先的**:配置只存在通道里(通道 + 推送目标两张表),
// 执行按 channel id 进行;调用方不重传规则,也不回传服务端的临时路径
// (apply 产出的执行记录里服务端自己记着产物路径)。
//
// 命令行/API 用 `histsync` 而非 `history`:`history` 已被
// 「同步执行历史」占用(见 internal/cli/commands/domains.go)。

type (
	// HistoryChannelInput 通道的创建/更新入参。
	HistoryChannelInput = coreservice.HistoryChannelInput
	// HistoryChannelDTO 通道的响应视图。
	HistoryChannelDTO = coreservice.HistoryChannelDTO
	// HistoryTargetInput 推送目标的创建/更新入参。
	HistoryTargetInput = coreservice.HistoryTargetInput
	// HistoryTargetDTO 推送目标的响应视图(不回显凭据)。
	HistoryTargetDTO = coreservice.HistoryTargetDTO
	// HistoryPlanDTO 同步计划的精简视图。
	HistoryPlanDTO = coreservice.HistoryPlanDTO
	// HistoryRunDTO 执行记录的响应视图。
	HistoryRunDTO = coreservice.HistoryRunDTO
	// HistoryPushDTO 推送结果。
	HistoryPushDTO = coreservice.HistoryPushDTO
	// HistoryRollbackDTO 回滚结果。
	HistoryRollbackDTO = coreservice.HistoryRollbackDTO
	// HistoryChainResult 通道内执行记录链的校验结果。
	HistoryChainResult = coreservice.HistoryChainResult
	// HistoryOverrideInput 裁决入参。
	HistoryOverrideInput = coreservice.HistoryOverrideInput
	// HistoryOverrideDTO 裁决响应视图。
	HistoryOverrideDTO = coreservice.HistoryOverrideDTO
	// PreviewReport 预检报告。
	PreviewReport = coreservice.PreviewReport
	// GateReport 发布前门禁结果。
	GateReport = coreservice.GateReport
	// TargetImpact 推送目标的影响面。
	TargetImpact = coreservice.TargetImpact
	// CredentialStatus 凭据状态。
	CredentialStatus = coreservice.CredentialStatus
	// HistoryCredentialInput 凭据轮换入参。
	HistoryCredentialInput = coreservice.HistoryCredentialInput
	// HistorySyncOptions 一次同步的选项。
	HistorySyncOptions = coreservice.HistorySyncOptions
	// HistorySyncReport 一次同步的结果。
	HistorySyncReport = coreservice.HistorySyncReport

	// PathRuleDTO 路径规则。
	PathRuleDTO = coreservice.PathRuleDTO
	// ScrubRuleDTO 痕迹规则。
	ScrubRuleDTO = coreservice.ScrubRuleDTO

	// FilePlan 单个文件的处置计划。
	FilePlan = history.FilePlan
	// CommitPlan 按源侧提交分组的回滚视图。
	CommitPlan = history.CommitPlan
	// Resolution hold 文件的解法。
	Resolution = history.Resolution
	// ConflictDossier 待人工/AI 裁决的冲突卷宗。
	ConflictDossier = history.ConflictDossier
	// HistorySummary 计划统计。
	HistorySummary = history.Summary
	// ScrubHit 痕迹命中项。
	ScrubHit = history.ScrubHit
)
