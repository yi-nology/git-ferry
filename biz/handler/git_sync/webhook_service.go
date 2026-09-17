package git_sync

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/webhook"
	"github.com/yi-nology/git-ferry/internal/converter"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

func RuleList(ctx context.Context, c *app.RequestContext) {
	var req webhook.ListRulesReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repoKey is required")
		return
	}

	list, err := GetSyncService().ListRules(ctx, req.RepoKey)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, &webhook.ListRulesResp{Rules: converter.ToRuleInfoList(list)})
}

func RuleGet(ctx context.Context, c *app.RequestContext) {
	var req webhook.GetRuleReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID <= 0 {
		response.BadRequest(c, "id is required")
		return
	}
	r, err := GetSyncService().GetRule(ctx, uint(req.ID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if r == nil {
		response.NotFound(c, "rule not found")
		return
	}
	response.Success(c, &webhook.GetRuleResp{Rule: converter.ToRuleInfo(r)})
}

// parseTaskKeys 解析逗号分隔的任务 key 列表(去空白、去空项)。
func parseTaskKeys(s string) []string {
	if s == "" {
		return nil
	}
	var keys []string
	for _, key := range strings.Split(s, ",") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func RuleCreate(ctx context.Context, c *app.RequestContext) {
	var req webhook.CreateRuleReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.RepoKey == "" {
		response.BadRequest(c, "name and repoKey are required")
		return
	}

	taskKeys := parseTaskKeys(req.SyncTaskKeys)

	r, err := GetSyncService().CreateRule(ctx, &corebridge.CreateRuleRequest{
		Name: req.Name, RepoKey: req.RepoKey, EventType: req.EventType,
		BranchPattern: req.BranchPattern, Action: req.Action,
		TaskKeys: taskKeys, MinInterval: int(req.MinInterval),
		Enabled: req.Enabled, Description: req.Description,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "create", "rule", r.Name, "创建 webhook 规则 "+r.Name)
	response.Created(c, &webhook.CreateRuleResp{Rule: converter.ToRuleInfo(r)})
}

func RuleUpdate(ctx context.Context, c *app.RequestContext) {
	var req webhook.UpdateRuleReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID <= 0 {
		response.BadRequest(c, "id is required")
		return
	}

	taskKeys := parseTaskKeys(req.SyncTaskKeys)

	r, err := GetSyncService().UpdateRule(ctx, &corebridge.UpdateRuleRequest{
		ID: uint(req.ID), Name: req.Name, EventType: req.EventType,
		BranchPattern: req.BranchPattern, Action: req.Action,
		TaskKeys: taskKeys, MinInterval: int(req.MinInterval),
		Enabled: req.Enabled, Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, corebridge.ErrRuleNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "update", "rule", r.Name, "更新 webhook 规则 "+r.Name)
	response.Success(c, &webhook.UpdateRuleResp{Rule: converter.ToRuleInfo(r)})
}

func RuleDelete(ctx context.Context, c *app.RequestContext) {
	var req webhook.DeleteRuleReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID <= 0 {
		response.BadRequest(c, "id is required")
		return
	}
	if err := GetSyncService().DeleteRule(ctx, uint(req.ID)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "delete", "rule", strconv.FormatUint(uint64(req.ID), 10), "删除 webhook 规则 #"+strconv.FormatUint(uint64(req.ID), 10))
	response.NoContent(c)
}

func ListEvents(ctx context.Context, c *app.RequestContext) {
	var req webhook.ListEventsReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repoKey is required")
		return
	}

	offset, _ := strconv.Atoi(c.Query("offset"))
	page := corebridge.DefaultPagination(offset, int(req.Limit))
	events, total, err := GetSyncService().ListEvents(ctx, req.RepoKey, page.Offset, page.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, converter.ToEventInfoList(events), total, page.Offset/page.Limit+1, page.Limit)
}

func RetryEvent(ctx context.Context, c *app.RequestContext) {
	var req webhook.RetryEventReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID <= 0 {
		response.BadRequest(c, "id is required")
		return
	}
	if err := GetSyncService().RetryEvent(ctx, uint(req.ID)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "retry", "event", strconv.FormatUint(uint64(req.ID), 10), "重试 webhook 事件 #"+strconv.FormatUint(uint64(req.ID), 10))
	response.Success(c, &webhook.RetryEventResp{Success: true, Message: "event retried"})
}

// RegisterPlatformWebhook 在平台侧为仓库注册 Webhook。
// @router /api/v1/webhook/platform/register [POST]
func RegisterPlatformWebhook(ctx context.Context, c *app.RequestContext) {
	var req webhook.RegisterPlatformWebhookReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" || req.CallbackUrl == "" {
		response.BadRequest(c, "repo_key and callback_url are required")
		return
	}

	wh, err := GetSyncService().RegisterPlatformWebhook(ctx, req.RepoKey, req.CallbackUrl, req.Secret, req.Events)
	if err != nil {
		if errors.Is(err, corebridge.ErrRepoNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "create", "platform_webhook", req.RepoKey, "为仓库 "+req.RepoKey+" 注册平台 Webhook #"+strconv.FormatInt(wh.ID, 10))
	response.Created(c, &webhook.RegisterPlatformWebhookResp{
		Webhook: &webhook.PlatformWebhookInfo{ID: wh.ID, URL: wh.URL, Events: wh.Events},
	})
}

// ListPlatformWebhooks 列出平台侧已注册的 Webhook。
// @router /api/v1/webhook/platform/list [GET]
func ListPlatformWebhooks(ctx context.Context, c *app.RequestContext) {
	var req webhook.ListPlatformWebhooksReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}

	webhooks, err := GetSyncService().ListPlatformWebhooks(ctx, req.RepoKey)
	if err != nil {
		if errors.Is(err, corebridge.ErrRepoNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	list := make([]*webhook.PlatformWebhookInfo, 0, len(webhooks))
	for _, wh := range webhooks {
		list = append(list, &webhook.PlatformWebhookInfo{ID: wh.ID, URL: wh.URL, Events: wh.Events})
	}
	response.Success(c, &webhook.ListPlatformWebhooksResp{Webhooks: list})
}

// DeletePlatformWebhook 删除平台侧的 Webhook。
// @router /api/v1/webhook/platform/delete [POST]
func DeletePlatformWebhook(ctx context.Context, c *app.RequestContext) {
	var req webhook.DeletePlatformWebhookReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" || req.WebhookId <= 0 {
		response.BadRequest(c, "repo_key and webhook_id are required")
		return
	}

	if err := GetSyncService().DeletePlatformWebhook(ctx, req.RepoKey, req.WebhookId); err != nil {
		if errors.Is(err, corebridge.ErrRepoNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "delete", "platform_webhook", req.RepoKey, "删除仓库 "+req.RepoKey+" 的平台 Webhook #"+strconv.FormatInt(req.WebhookId, 10))
	response.NoContent(c)
}
