package corebridge

import (
	"github.com/yi-nology/git-sync-core/dao"
	"github.com/yi-nology/git-sync-core/model"
	coreservice "github.com/yi-nology/git-sync-core/service"
)

// ===== 镜像中心(开源发布 / 仓库备份) =====

type Pagination = dao.Pagination

// ===== 镜像中心(开源发布 / 仓库备份) =====

type (
	MirrorChannel            = model.MirrorChannel
	MirrorTarget             = model.MirrorTarget
	MirrorRun                = model.MirrorRun
	MirrorStep               = model.MirrorStep
	TagStatus                = model.TagStatus
	MirrorVersionsResult     = coreservice.MirrorVersionsResult
	MirrorVersion            = coreservice.MirrorVersion
	MirrorVersionTarget      = coreservice.MirrorVersionTarget
	CreateMirrorChannelInput = coreservice.CreateMirrorChannelInput
	MirrorTargetInput        = coreservice.MirrorTargetInput
	ExecuteMirrorRunInput    = coreservice.ExecuteMirrorRunInput
)

// 镜像常量
const (
	MirrorModePublish = model.MirrorModePublish
	MirrorModeBackup  = model.MirrorModeBackup
	MirrorRunPending  = model.MirrorRunPending
	MirrorRunRunning  = model.MirrorRunRunning
	MirrorRunSuccess  = model.MirrorRunSuccess
	MirrorRunFailed   = model.MirrorRunFailed
	MirrorKindVerify  = model.MirrorKindVerify
	MirrorKindPublish = model.MirrorKindPublish
)
