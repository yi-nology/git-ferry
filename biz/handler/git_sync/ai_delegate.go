package git_sync

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// AIDelegateStatus GET /api/v1/ai/delegate/{task_id} —— 查一次外部 agent 委派的状态。
//
// **为什么需要这个接口**:委派是**异步**的 —— 确认后工具立刻返回任务 ID,
// 外部 agent 在本机子进程里跑(可能几分钟)。前端要能轮询到结果,
// 而不是干等「已派出」。
//
// 角色:与其它 AI 端点一致(读自己的委派任务);任务结果里可能有仓库名等运维信息,
// 沿用 AI 端点既有的鉴权强度(登录 + API Key)。
func AIDelegateStatus(ctx context.Context, c *app.RequestContext) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		response.BadRequest(c, "task_id 不能为空")
		return
	}
	r := getAgentRunner()
	if r == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_disabled")
		return
	}
	snap, ok := r.DelegateStatus(taskID)
	if !ok {
		response.Error(c, http.StatusNotFound, "delegate_task_not_found")
		return
	}
	response.Success(c, snap)
}
