package git_sync

import (
	"context"
	"strconv"
	"strings"

	mirrormodel "github.com/yi-nology/git-ferry/biz/model/mirror"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/pkg/convert"
	"github.com/yi-nology/git-ferry/internal/converter"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 请求体 =====

// ===== 辅助 =====

func mirrorID(c *app.RequestContext) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}

// parsePage 解析分页参数(缺省 1/50,上限 200)。
// 与其它列表端点共用 converter.PageToOffset——此前此处独立实现且缺省值是 20,
// 于是同一次请求在任务/仓库列表返回 50 条、在镜像通道列表返回 20 条。
func parsePage(c *app.RequestContext) corebridge.Pagination {
	offset, limit := converter.PageToOffset(
		int32(convert.Str2Int(c.Query("page"), 0)),
		int32(convert.Str2Int(c.Query("pageSize"), 0)),
	)
	return corebridge.DefaultPagination(offset, limit)
}

func toMirrorTargetInputs(reqs []*mirrormodel.MirrorTargetReq) []corebridge.MirrorTargetInput {
	inputs := make([]corebridge.MirrorTargetInput, 0, len(reqs))
	for _, r := range reqs {
		if r == nil {
			continue
		}
		inputs = append(inputs, corebridge.MirrorTargetInput{
			Remote:       strings.TrimSpace(r.Remote),
			RepoURL:      strings.TrimSpace(r.RepoUrl),
			TargetModule: strings.TrimSpace(r.TargetModule),
			CredType:     r.CredType,
			Credential:   r.Credential,
			Username:     r.Username,
			// SSH 主机密钥钉扎:目标侧之前没法钉 ——
			// 于是同一次发布里「检查路径」fail-closed、「推送路径」TOFU。
			InsecureSkipTLS:    r.InsecureSkipTLS,
			HostKeyFingerprint: strings.TrimSpace(r.HostKeyFingerprint),
			KnownHostsPath:     strings.TrimSpace(r.KnownHostsPath),
		})
	}
	return inputs
}

// toMirrorScrubRuleInputs 把痕迹规则入参转成 core 的入参。
//
// 痕迹规则在**创建通道时**就校验(正则编不过直接拒),
// 不会拖到发布那一刻才炸。
func toMirrorScrubRuleInputs(reqs []*mirrormodel.MirrorScrubRuleReq) []corebridge.MirrorScrubRuleInput {
	out := make([]corebridge.MirrorScrubRuleInput, 0, len(reqs))
	for _, r := range reqs {
		if r == nil {
			continue
		}
		out = append(out, corebridge.MirrorScrubRuleInput{
			Name:        strings.TrimSpace(r.Name),
			Category:    strings.TrimSpace(r.Category),
			Pattern:     r.Pattern,
			Severity:    strings.TrimSpace(r.Severity),
			Replacement: r.Replacement,
		})
	}
	return out
}

// ===== 通道 =====

func MirrorChannelList(ctx context.Context, c *app.RequestContext) {
	page := parsePage(c)
	channels, total, err := GetSyncService().Mirror().ListMirrorChannels(page)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{"list": channels, "total": total})
}

func MirrorChannelCreate(ctx context.Context, c *app.RequestContext) {
	var req mirrormodel.CreateMirrorChannelReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ch, err := GetSyncService().Mirror().CreateMirrorChannel(ctx, corebridge.CreateMirrorChannelInput{
		Name:       req.Name,
		Mode:       req.Mode,
		RepoKey:    req.RepoKey,
		Targets:    toMirrorTargetInputs(req.Targets),
		ScrubRules: toMirrorScrubRuleInputs(req.ScrubRules),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func MirrorChannelGet(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	ch, err := GetSyncService().Mirror().GetMirrorChannel(id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func MirrorChannelDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	if err := GetSyncService().Mirror().DeleteMirrorChannel(c.Query("confirm"), id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]bool{"deleted": true})
}

// ===== 目标 =====

func MirrorTargetUpdate(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	var req mirrormodel.UpdateMirrorTargetReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	t, err := GetSyncService().Mirror().UpdateMirrorTarget(ctx, id, corebridge.MirrorTargetInput{
		Remote:       req.Remote,
		RepoURL:      req.RepoUrl,
		TargetModule: req.TargetModule,
		CredType:     req.CredType,
		Credential:   req.Credential,
		Username:     req.Username,
		// SSH 主机密钥钉扎(与创建同一套字段)。
		InsecureSkipTLS:    req.InsecureSkipTLS,
		HostKeyFingerprint: strings.TrimSpace(req.HostKeyFingerprint),
		KnownHostsPath:     strings.TrimSpace(req.KnownHostsPath),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, t)
}

func MirrorTargetDelete(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	if err := GetSyncService().Mirror().DeleteMirrorTarget(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]bool{"deleted": true})
}

func MirrorTargetTest(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	if err := GetSyncService().Mirror().TestMirrorTarget(ctx, id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]bool{"ok": true})
}

// ===== 版本矩阵 / 预检 / 执行 / 验证 =====

func MirrorVersions(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	result, err := GetSyncService().Mirror().GetMirrorVersions(ctx, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func MirrorPreview(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	var req mirrormodel.MirrorPreviewReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Tag == "" {
		response.BadRequest(c, "tag is required")
		return
	}
	rep, err := GetSyncService().Mirror().PreviewMirrorRun(ctx, id, uint(req.TargetId), req.Tag)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, rep)
}

func MirrorRunCreate(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	var req mirrormodel.MirrorRunReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	run, err := GetSyncService().Mirror().ExecuteMirrorRun(ctx, id, corebridge.ExecuteMirrorRunInput{
		TargetID:       uint(req.TargetId),
		Tags:           req.Tags,
		AllowOverwrite: req.AllowOverwrite,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, run)
}

func MirrorRunList(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	page := parsePage(c)
	runs, total, err := GetSyncService().Mirror().ListMirrorRuns(id, page)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// 列表剥掉 Report/Steps 等大 TEXT 字段(单条可达数百 KB),详情接口保留全量
	light := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		if r == nil {
			continue
		}
		light = append(light, map[string]any{
			"id": r.ID, "channelId": r.ChannelID, "targetId": r.TargetID,
			"kind": r.Kind, "tags": r.Tags, "status": r.Status,
			"allowOverwrite": r.AllowOverwrite,
			"startedAt":      r.StartedAt, "finishedAt": r.FinishedAt,
			"error": textutil.TruncateRunes(r.Error, 300),
		})
	}
	response.Success(c, map[string]any{"list": light, "total": total})
}

func MirrorRunGet(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	run, err := GetSyncService().Mirror().GetMirrorRun(id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, run)
}

func MirrorVerify(ctx context.Context, c *app.RequestContext) {
	id, ok := mirrorID(c)
	if !ok {
		return
	}
	var req mirrormodel.MirrorVerifyReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Tag == "" {
		response.BadRequest(c, "tag is required")
		return
	}
	run, err := GetSyncService().Mirror().VerifyMirrorTargetVersion(ctx, id, uint(req.TargetId), req.Tag)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, run)
}
