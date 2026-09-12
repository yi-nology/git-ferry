package git_sync

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	platform "github.com/yi-nology/git-sync-service/biz/model/platform"
	"github.com/yi-nology/git-sync-service/internal/converter"
	"github.com/yi-nology/git-sync-service/internal/corebridge"
	"github.com/yi-nology/git-sync-service/internal/pkg/response"
)

// CreatePlatform 创建平台
func CreatePlatform(ctx context.Context, c *app.RequestContext) {
	var req platform.CreatePlatformReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.Type == "" || req.AccessToken == "" {
		response.BadRequest(c, "name, type, access_token are required")
		return
	}
	if !corebridge.ValidPlatformTypes[req.Type] {
		response.BadRequest(c, fmt.Sprintf("unsupported platform type: %s (valid: github, gitlab, gitea, gitee, gitcode, atomgit, tencent_code, custom)", req.Type))
		return
	}

	// 生成 API URL（如果未提供）
	apiURL := req.ApiUrl
	if apiURL == "" && req.Type != corebridge.PlatformTypeCustom {
		apiURL = corebridge.GetAPIURL(req.Type, req.InstanceUrl)
	}

	p := &corebridge.Platform{
		Key:           uuid.New().String(),
		Name:          req.Name,
		Type:          req.Type,
		InstanceURL:   req.InstanceUrl,
		APIURL:        apiURL,
		AccessToken:   req.AccessToken,
		SkipTLSVerify: req.SkipTlsVerify,
		CACertPath:    req.CaCertPath,
		ProxyURL:      req.ProxyUrl,
		IsDefault:     req.IsDefault,
		Status:        corebridge.PlatformStatusActive,
	}

	if err := GetSyncService().CreatePlatform(ctx, p); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "create", "platform", p.Key, "创建平台 "+p.Name)
	response.Created(c, &platform.CreatePlatformResp{
		Platform: converter.ToPlatformInfo(p),
	})
}

// GetPlatform 获取单个平台
func GetPlatform(ctx context.Context, c *app.RequestContext) {
	var req platform.GetPlatformReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	p, err := GetSyncService().GetPlatform(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if p == nil {
		response.NotFound(c, "platform not found")
		return
	}

	response.Success(c, &platform.GetPlatformResp{
		Platform: converter.ToPlatformInfo(p),
	})
}

// ListPlatforms 列出所有平台
func ListPlatforms(ctx context.Context, c *app.RequestContext) {
	platforms, err := GetSyncService().ListPlatforms(ctx)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, &platform.ListPlatformsResp{
		Platforms: converter.ToPlatformList(platforms),
		Total:     int64(len(platforms)),
	})
}

// UpdatePlatform 更新平台
func UpdatePlatform(ctx context.Context, c *app.RequestContext) {
	var req platform.UpdatePlatformReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	p, err := GetSyncService().GetPlatform(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if p == nil {
		response.NotFound(c, "platform not found")
		return
	}

	// 更新字段（空值表示不更新）
	converter.ApplyPlatformUpdate(p, &req)

	if err := GetSyncService().UpdatePlatform(ctx, p); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "update", "platform", req.Key, "更新平台 "+p.Name)
	response.Success(c, &platform.UpdatePlatformResp{
		Platform: converter.ToPlatformInfo(p),
	})
}

// DeletePlatform 删除平台
func DeletePlatform(ctx context.Context, c *app.RequestContext) {
	var req platform.DeletePlatformReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	// 检查是否有关联的仓库(用 COUNT 而非全量加载)
	repoCount, err := GetSyncService().CountReposByPlatform(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if repoCount > 0 {
		response.BadRequest(c, "该平台下还有仓库，请先删除关联的仓库")
		return
	}

	if err := GetSyncService().DeletePlatform(ctx, req.Key); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "delete", "platform", req.Key, "删除平台 "+req.Key)
	response.NoContent(c)
}

// SetDefaultPlatform 设置默认平台
func SetDefaultPlatform(ctx context.Context, c *app.RequestContext) {
	var req platform.SetDefaultPlatformReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	if err := GetSyncService().SetDefaultPlatform(ctx, req.Key); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "update", "platform", req.Key, "设置默认平台 "+req.Key)
	response.Success(c, &platform.SetDefaultPlatformResp{
		Message: "设置成功",
	})
}

// TestPlatformConnection 测试平台连接
func TestPlatformConnection(ctx context.Context, c *app.RequestContext) {
	var req platform.TestPlatformConnectionReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	result, err := GetSyncService().TestPlatformConnection(ctx, req.Key)
	if err != nil {
		// 更新平台状态为错误
		if statusErr := GetSyncService().UpdatePlatformStatus(ctx, req.Key, corebridge.PlatformStatusError, err.Error()); statusErr != nil {
			slog.Warn("failed to update platform status after test error", "key", req.Key, "error", statusErr)
		}
		response.InternalError(c, err.Error())
		return
	}

	// 按实际连接结果落库:connected=false 时记录失败详情,不能一律写成功
	if result != nil && result.Connected {
		if statusErr := GetSyncService().UpdatePlatformStatus(ctx, req.Key, corebridge.PlatformStatusActive, "connection successful"); statusErr != nil {
			slog.Warn("failed to update platform status to active", "key", req.Key, "error", statusErr)
		}
	} else {
		failMsg := "connection failed"
		if result != nil && result.Message != "" {
			failMsg = result.Message
		}
		if statusErr := GetSyncService().UpdatePlatformStatus(ctx, req.Key, corebridge.PlatformStatusError, failMsg); statusErr != nil {
			slog.Warn("failed to update platform status to error", "key", req.Key, "error", statusErr)
		}
	}

	response.Success(c, map[string]interface{}{
		"result": result,
	})
}

// ListPlatformRepos 列出平台上的仓库（从远程 API 获取）
func ListPlatformRepos(ctx context.Context, c *app.RequestContext) {
	var req platform.ListPlatformReposReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	page := req.Page
	if page == "" {
		page = "1"
	}
	perPage := req.PerPage
	if perPage == "" {
		perPage = "30"
	}

	repos, err := GetSyncService().ListPlatformRepos(ctx, req.Key, page, perPage)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// SDK 不回传远端真实总数,total 语义为"当页条数";
	// has_more 提示调用方还有下一页(返回条数打满请求页大小)。
	reqPerPage, _ := strconv.Atoi(perPage)
	response.Success(c, map[string]interface{}{
		"repos":    repos,
		"total":    len(repos),
		"has_more": reqPerPage > 0 && len(repos) == reqPerPage,
		"page":     page,
		"per_page": perPage,
	})
}

// SyncPlatformRepos 同步平台仓库到本地
func SyncPlatformRepos(ctx context.Context, c *app.RequestContext) {
	var req platform.SyncPlatformReposReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	count, err := GetSyncService().SyncPlatformRepos(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "sync", "platform", req.Key, fmt.Sprintf("同步平台仓库 %s（导入 %d 个）", req.Key, count))
	response.Success(c, &platform.SyncPlatformReposResp{
		Message:     "同步成功",
		SyncedCount: converter.SafeIntToInt32(count),
	})
}
