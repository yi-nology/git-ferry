package git_sync

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	sdkprov "github.com/yi-nology/git-platform-sdk/provider"
	"github.com/yi-nology/git-sync-service/biz/model/repo"
	"github.com/yi-nology/git-sync-service/internal/converter"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/internal/pkg/response"
	"github.com/yi-nology/git-sync-service/internal/service"
	syncmodel "github.com/yi-nology/git-sync-service/sync/model"
)

func RepoList(ctx context.Context, c *app.RequestContext) {
	var req repo.ListReposReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Apply defaults
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	if req.SortBy == "" {
		req.SortBy = "created_at"
	}
	if req.SortOrder == "" {
		req.SortOrder = "desc"
	}

	offset, limit := converter.PageToOffset(req.Page, req.PageSize)

	filter := dao.RepoFilter{
		Search:   req.Search,
		Platform: req.Platform,
		Status:   req.Status,
		SortBy:   req.SortBy,
		OrderBy:  req.SortOrder,
	}

	list, total, err := GetSyncService().ListReposWithFilter(ctx, offset, limit, &filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, converter.ToRepoInfoList(list), total, int(req.Page), int(req.PageSize))
}

func RepoGet(ctx context.Context, c *app.RequestContext) {
	var req repo.GetRepoReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	r, err := GetSyncService().GetRepo(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if r == nil {
		response.NotFound(c, "repo not found")
		return
	}
	response.Success(c, &repo.GetRepoResp{Repo: converter.ToRepoInfo(r)})
}

func RepoCreate(ctx context.Context, c *app.RequestContext) {
	var req repo.CreateRepoReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.RemoteUrl == "" {
		response.BadRequest(c, "name and remoteUrl are required")
		return
	}

	// Get platform_id from query parameter
	platformIDStr := c.Query("platform_id")
	var platformID uint
	if platformIDStr != "" {
		pid, err := strconv.ParseUint(platformIDStr, 10, 64)
		if err != nil {
			response.BadRequest(c, "invalid platform_id: must be a positive integer")
			return
		}
		platformID = uint(pid)
	}

	r, err := GetSyncService().CreateRepo(ctx, &syncmodel.CreateRepoRequest{
		Name: req.Name, RemoteURL: req.RemoteUrl, AccessToken: req.AccessToken, PlatformID: platformID,
	})
	if err != nil {
		if sdkprov.IsInvalidInput(err) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, fmt.Sprintf("create repo failed: %v", err))
		return
	}
	recordAudit(ctx, c, "create", "repo", r.Key, "创建仓库 "+r.Name)
	response.Created(c, &repo.CreateRepoResp{Repo: converter.ToRepoInfo(r)})
}

func RepoUpdate(ctx context.Context, c *app.RequestContext) {
	var req repo.UpdateRepoReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	r, err := GetSyncService().UpdateRepo(ctx, &syncmodel.UpdateRepoRequest{
		Key: req.Key, Name: req.Name, AccessToken: req.AccessToken,
	})
	if err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "update", "repo", r.Key, "更新仓库 "+r.Key)
	response.Success(c, &repo.UpdateRepoResp{Repo: converter.ToRepoInfo(r)})
}

func RepoDelete(ctx context.Context, c *app.RequestContext) {
	var req repo.DeleteRepoReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	if err := GetSyncService().DeleteRepo(ctx, req.Key); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "delete", "repo", req.Key, "删除仓库 "+req.Key)
	response.NoContent(c)
}

func RepoTest(ctx context.Context, c *app.RequestContext) {
	var req repo.TestConnectionReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	result, err := GetSyncService().TestConnection(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, &repo.TestConnectionResp{Success: result.Success, Message: result.Message})
}

func RepoBranches(ctx context.Context, c *app.RequestContext) {
	var req repo.ListBranchesReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	branches, err := GetSyncService().ListBranches(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, &repo.ListBranchesResp{Branches: branches})
}

// BatchRepos 批量操作仓库（目前仅支持 delete）。
func BatchRepos(ctx context.Context, c *app.RequestContext) {
	var req repo.BatchReposReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Action == "" {
		response.BadRequest(c, "action is required")
		return
	}
	if len(req.Keys) == 0 {
		response.BadRequest(c, "keys must not be empty")
		return
	}
	if len(req.Keys) > 200 {
		response.BadRequest(c, "too many keys: max 200 per batch")
		return
	}
	if req.Action != "delete" {
		response.BadRequest(c, fmt.Sprintf("unsupported action: %s", req.Action))
		return
	}

	total := len(req.Keys)
	success := 0
	failed := 0
	var errs []string
	for _, key := range req.Keys {
		if err := GetSyncService().DeleteRepo(ctx, key); err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("%s: %v", key, err))
		} else {
			success++
		}
	}
	resp := &repo.BatchReposResp{
		Total:   converter.SafeIntToInt32(total),
		Success: converter.SafeIntToInt32(success),
		Failed:  converter.SafeIntToInt32(failed),
		Errors:  errs,
	}
	// 全部失败返 400,部分失败返 207 Multi-Status,全部成功返 200
	if failed > 0 && success == 0 {
		response.Error(c, 400, "all deletions failed")
	} else if failed > 0 {
		c.JSON(207, resp)
	} else {
		response.Success(c, resp)
	}
}
