package git_sync

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-sync-service/internal/corebridge"
	"github.com/yi-nology/git-sync-service/internal/pkg/response"
)

// ===== 请求体 =====

type MirrorTargetReq struct {
	Remote       string `json:"remote"`
	RepoURL      string `json:"repoUrl"`
	TargetModule string `json:"targetModule"`
	CredType     string `json:"credType"`
	Credential   string `json:"credential"`
	Username     string `json:"username"`
}

type CreateMirrorChannelReq struct {
	Name    string            `json:"name"`
	Mode    string            `json:"mode"`
	RepoKey string            `json:"repoKey"`
	Targets []MirrorTargetReq `json:"targets"`
}

type UpdateMirrorTargetReq struct {
	MirrorTargetReq
}

type MirrorPreviewReq struct {
	TargetID uint   `json:"targetId"`
	Tag      string `json:"tag"`
}

type MirrorRunReq struct {
	TargetID       uint     `json:"targetId"`
	Tags           []string `json:"tags"`
	AllowOverwrite bool     `json:"allowOverwrite"`
}

type MirrorVerifyReq struct {
	TargetID uint   `json:"targetId"`
	Tag      string `json:"tag"`
}

// ===== 辅助 =====

func mirrorID(c *app.RequestContext) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}

// parsePage 解析分页参数(缺省 1/20,上限 200)。
func parsePage(c *app.RequestContext) corebridge.Pagination {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return corebridge.DefaultPagination((page-1)*pageSize, pageSize)
}

func toMirrorTargetInputs(reqs []MirrorTargetReq) []corebridge.MirrorTargetInput {
	inputs := make([]corebridge.MirrorTargetInput, 0, len(reqs))
	for _, r := range reqs {
		inputs = append(inputs, corebridge.MirrorTargetInput{
			Remote:       strings.TrimSpace(r.Remote),
			RepoURL:      strings.TrimSpace(r.RepoURL),
			TargetModule: strings.TrimSpace(r.TargetModule),
			CredType:     r.CredType,
			Credential:   r.Credential,
			Username:     r.Username,
		})
	}
	return inputs
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
	var req CreateMirrorChannelReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ch, err := GetSyncService().Mirror().CreateMirrorChannel(ctx, corebridge.CreateMirrorChannelInput{
		Name:    req.Name,
		Mode:    req.Mode,
		RepoKey: req.RepoKey,
		Targets: toMirrorTargetInputs(req.Targets),
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
	var req UpdateMirrorTargetReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	t, err := GetSyncService().Mirror().UpdateMirrorTarget(ctx, id, corebridge.MirrorTargetInput{
		Remote:       req.Remote,
		RepoURL:      req.RepoURL,
		TargetModule: req.TargetModule,
		CredType:     req.CredType,
		Credential:   req.Credential,
		Username:     req.Username,
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
	var req MirrorPreviewReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Tag == "" {
		response.BadRequest(c, "tag is required")
		return
	}
	rep, err := GetSyncService().Mirror().PreviewMirrorRun(ctx, id, req.TargetID, req.Tag)
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
	var req MirrorRunReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	run, err := GetSyncService().Mirror().ExecuteMirrorRun(ctx, id, corebridge.ExecuteMirrorRunInput{
		TargetID:       req.TargetID,
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
	response.Success(c, map[string]any{"list": runs, "total": total})
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
	var req MirrorVerifyReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Tag == "" {
		response.BadRequest(c, "tag is required")
		return
	}
	run, err := GetSyncService().Mirror().VerifyMirrorTargetVersion(ctx, id, req.TargetID, req.Tag)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, run)
}
