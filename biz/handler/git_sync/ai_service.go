package git_sync

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yi-nology/git-ferry/biz/model/ai"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	hertzsse "github.com/hertz-contrib/sse"
	"github.com/oekit/pkg/ptrx"

	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/pkg/response"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

var (
	agentRunnerMu sync.RWMutex
	agentRunnerFn func() *agent.Runner
)

// SetAgentRunner 注入 AI Runner(nil = AI 未启用)。
func SetAgentRunner(fn func() *agent.Runner) {
	agentRunnerMu.Lock()
	defer agentRunnerMu.Unlock()
	agentRunnerFn = fn
}

func getAgentRunner() *agent.Runner {
	agentRunnerMu.RLock()
	fn := agentRunnerFn
	agentRunnerMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// AIStatus GET /api/v1/ai/status —— 前端探测 AI 是否可用。
func AIStatus(ctx context.Context, c *app.RequestContext) {
	r := getAgentRunner()
	if r == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_disabled")
		return
	}
	// 后端信息:界面要能看出「现在跟谁说话」(内置助手 / 外部 coding agent),
	// 以及外部后端**装没装**(静态就位性,快;真正的握手在「测试连接」里做)。
	ready, readyMsg := r.Ready()
	response.Success(c, map[string]any{
		"enabled":       true,
		"model":         r.ModelName(),
		"backend":       r.BackendID(),
		"backend_label": r.BackendLabel(),
		"ready":         ready,
		"ready_msg":     readyMsg,
	})
}

// AIChat POST /api/v1/ai/chat —— SSE 流式响应。
// 事件:start(含 session_id)→ delta* / tool_start / tool_end / tool_confirm → done|error。
func AIChat(ctx context.Context, c *app.RequestContext) {
	// 角色**跟着 ctx 往后走**:助手链路的工具是进程内直调 core service,
	// 不过路由守卫 —— 工具层要自己判角色(尤其「强制推送」要 admin)。
	ctx = authz.WithRole(ctx, GetAuthRole(c))
	r := getAgentRunner()
	if r == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_disabled")
		return
	}
	var req ai.AIChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 校验前置:先建会话后校验会让空消息请求留下垃圾会话。
	// 确认与授权两条回传路径都不带业务消息(消息仅要求非空占位)。
	if req.ConfirmedToolCall == nil && req.PermissionCall == nil && strings.TrimSpace(req.Message) == "" {
		response.BadRequest(c, "message 不能为空")
		return
	}

	// 授权回传路径(外部 agent 的 session/request_permission):
	// **不需要会话存在** —— 待授权状态在 agent 进程侧,不在 git-ferry 会话上。
	// 会话过期时也不该让用户点的授权白点。
	if req.PermissionCall != nil {
		err := r.RespondPermission(ctx, ptrx.Deref(req.SessionId, ""),
			req.PermissionCall.RequestId, req.PermissionCall.OptionId)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.Success(c, map[string]any{"ok": true})
		return
	}

	store := r.Sessions()
	var sess *agent.Session
	if ptrx.Deref(req.SessionId, "") == "" {
		sess = store.Create()
	} else {
		var err error
		sess, err = store.Get(ptrx.Deref(req.SessionId, ""))
		if err != nil {
			response.Error(c, consts.StatusNotFound, "会话不存在或已过期,请重新开始")
			return
		}
	}

	stream := hertzsse.NewStream(c)
	publish := func(ev agent.Event) bool {
		b, err := json.Marshal(ev)
		if err != nil {
			return true
		}
		if err := stream.Publish(&hertzsse.Event{Event: ev.Type, Data: b}); err != nil {
			return false // 客户端断开
		}
		return true
	}

	// 确认路径:校验令牌 → 直达执行 → tool_end/done,不经模型。
	if req.ConfirmedToolCall != nil {
		out, err := r.ExecuteConfirmed(ctx, sess, req.ConfirmedToolCall.Tool, req.ConfirmedToolCall.Token)
		if err != nil {
			if !publish(agent.Event{Type: "error", Content: confirmErrText(err)}) {
				return
			}
			publish(agent.Event{Type: "done"})
			return
		}
		publish(agent.Event{Type: "tool_end", Tool: req.ConfirmedToolCall.Tool, Result: textutil.TruncateRunes(out, 600)})
		publish(agent.Event{Type: "done"})
		return
	}

	events, cancel, err := r.Run(ctx, sess, req.Message)
	if err != nil {
		if errors.Is(err, agent.ErrBusy) {
			response.Error(c, consts.StatusTooManyRequests, agent.ErrBusy.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	defer cancel()
	for ev := range events {
		if !publish(ev) {
			cancel() // 客户端断开:立即终止模型流与事件发送,释放信号量,不再烧 token
			return
		}
	}
}

func confirmErrText(err error) string {
	switch {
	case errors.Is(err, agent.ErrNoPending), errors.Is(err, agent.ErrConfirmMismatch):
		return "确认请求无效或已过期,请重新发起"
	case errors.Is(err, agent.ErrConfirmExpired):
		return "确认已超时(5 分钟),请重新发起"
	default:
		return "执行失败: " + err.Error()
	}
}
