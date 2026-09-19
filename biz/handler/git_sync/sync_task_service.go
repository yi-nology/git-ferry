package git_sync

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/sync_task"
	"github.com/yi-nology/git-ferry/internal/converter"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

func TaskList(ctx context.Context, c *app.RequestContext) {
	var req sync_task.ListTasksReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	offset, limit := converter.PageToOffset(req.Page, req.PageSize)
	list, total, err := GetSyncService().ListTasks(ctx, req.RepoKey, offset, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, &sync_task.ListTasksResp{
		Tasks: converter.ToTaskInfoList(list),
		Total: total,
	})
}

func TaskGet(ctx context.Context, c *app.RequestContext) {
	var req sync_task.GetTaskReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	t, err := GetSyncService().GetTask(ctx, req.Key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if t == nil {
		response.NotFound(c, "task not found")
		return
	}
	response.Success(c, &sync_task.GetTaskResp{Task: converter.ToTaskInfo(t)})
}

func TaskCreate(ctx context.Context, c *app.RequestContext) {
	var req sync_task.CreateTaskReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.SourceRepoKey == "" || req.TargetRepoKey == "" {
		response.BadRequest(c, "name, sourceRepoKey and targetRepoKey are required")
		return
	}

	t, err := GetSyncService().CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: req.Name, SourceRepoKey: req.SourceRepoKey, SourceBranch: req.SourceBranch,
		TargetRepoKey: req.TargetRepoKey, TargetBranch: req.TargetBranch,
		SyncMode: req.SyncMode, Cron: req.Cron, GitTags: req.GitTags,
		GitForce: req.GitForce, GitPrune: req.GitPrune,
	})
	if err != nil {
		response.InternalError(c, fmt.Sprintf("create task failed: %v", err))
		return
	}
	recordAudit(ctx, c, "create", "task", t.Key, "创建同步任务 "+t.Name)
	response.Created(c, &sync_task.CreateTaskResp{Task: converter.ToTaskInfo(t)})
}

func TaskUpdate(ctx context.Context, c *app.RequestContext) {
	var req sync_task.UpdateTaskReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}

	// bool 字段转指针:壳层表单总是携带全部字段,此处显式转为指针
	// 传给 core(指针语义:非 nil 即"更新为该值")
	t, err := GetSyncService().UpdateTask(ctx, &corebridge.UpdateTaskRequest{
		Key: req.Key, Name: req.Name, SourceBranch: req.SourceBranch,
		TargetBranch: req.TargetBranch, SyncMode: req.SyncMode, Cron: req.Cron,
		Enabled: &req.Enabled, GitTags: &req.GitTags, GitForce: &req.GitForce,
		GitPrune: &req.GitPrune,
	})
	if err != nil {
		if errors.Is(err, corebridge.ErrTaskNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "update", "task", t.Key, "更新同步任务 "+t.Key)
	response.Success(c, &sync_task.UpdateTaskResp{Task: converter.ToTaskInfo(t)})
}

func TaskDelete(ctx context.Context, c *app.RequestContext) {
	var req sync_task.DeleteTaskReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}
	if err := GetSyncService().DeleteTask(ctx, req.Key); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "delete", "task", req.Key, "删除同步任务 "+req.Key)
	response.NoContent(c)
}

func TaskRun(ctx context.Context, c *app.RequestContext) {
	var req sync_task.RunTaskReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}
	// 异步受理:同步执行可能分钟级,HTTP 不等它跑完;校验(存在/启用/并发快速失败)仍同步返回。
	// 真实结果落执行历史,前端靠列表/历史刷新观察。
	if err := GetSyncService().RunTaskAsync(req.Key, corebridge.TriggerManual, nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "run", "task", req.Key, "手动触发同步 "+req.Key)
	response.Success(c, &sync_task.RunTaskResp{Success: true, Message: "task started"})
}

func TaskPreview(ctx context.Context, c *app.RequestContext) {
	var req sync_task.PreviewSyncReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.SourceRepoKey == "" || req.TargetRepoKey == "" {
		response.BadRequest(c, "sourceRepoKey and targetRepoKey are required")
		return
	}

	result, err := GetSyncService().PreviewSync(ctx, &corebridge.PreviewSyncRequest{
		SourceRepoKey: req.SourceRepoKey, SourceBranch: req.SourceBranch,
		TargetRepoKey: req.TargetRepoKey, TargetBranch: req.TargetBranch,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, &sync_task.PreviewSyncResp{
		CanSync: result.CanSync, SourceExists: result.SourceExists,
		TargetExists: result.TargetExists, Message: result.Message,
	})
}

func TaskHistory(ctx context.Context, c *app.RequestContext) {
	var req sync_task.ListHistoryReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.TaskKey == "" {
		response.BadRequest(c, "taskKey is required")
		return
	}

	offset, _ := strconv.Atoi(c.Query("offset"))
	page := corebridge.DefaultPagination(offset, int(req.Limit))
	runs, _, err := GetSyncService().ListHistory(ctx, req.TaskKey, page.Offset, page.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 列表视图用轻量转换器,省略 Details/Steps 减少 90%+ 响应体积
	response.Success(c, &sync_task.ListHistoryResp{Runs: converter.ToSyncRunInfoListLight(runs)})
}

// DeleteHistory 删除单条同步执行历史(失败记录清理用)。
func DeleteHistory(ctx context.Context, c *app.RequestContext) {
	var req sync_task.DeleteHistoryReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID <= 0 {
		response.BadRequest(c, "id is required")
		return
	}

	if err := GetSyncService().DeleteHistory(ctx, uint(req.ID)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	recordAudit(ctx, c, "delete", "history", strconv.FormatInt(req.ID, 10),
		fmt.Sprintf("删除同步历史记录 #%d", req.ID))
	response.Success(c, &sync_task.DeleteHistoryResp{Message: "删除成功"})
}
