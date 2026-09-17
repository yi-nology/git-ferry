package git_sync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	hertzsse "github.com/hertz-contrib/sse"

	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
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
	response.Success(c, map[string]any{"enabled": true, "model": r.ModelName()})
}

type aiChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Confirmed *struct {
		Tool  string `json:"tool"`
		Token string `json:"token"`
	} `json:"confirmed_tool_call"`
}

// AIChat POST /api/v1/ai/chat —— SSE 流式响应。
// 事件:start(含 session_id)→ delta* / tool_start / tool_end / tool_confirm → done|error。
func AIChat(ctx context.Context, c *app.RequestContext) {
	r := getAgentRunner()
	if r == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_disabled")
		return
	}
	var req aiChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	store := r.Sessions()
	var sess *agent.Session
	if req.SessionID == "" {
		sess = store.Create()
	} else {
		var err error
		sess, err = store.Get(req.SessionID)
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
	if req.Confirmed != nil {
		out, err := r.ExecuteConfirmed(ctx, sess, req.Confirmed.Tool, req.Confirmed.Token)
		if err != nil {
			if !publish(agent.Event{Type: "error", Content: confirmErrText(err)}) {
				return
			}
			publish(agent.Event{Type: "done"})
			return
		}
		publish(agent.Event{Type: "tool_end", Tool: req.Confirmed.Tool, Result: truncateRunes(out, 600)})
		publish(agent.Event{Type: "done"})
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		response.BadRequest(c, "message 不能为空")
		return
	}

	events, err := r.Run(ctx, sess, req.Message)
	if err != nil {
		if errors.Is(err, agent.ErrBusy) {
			response.Error(c, consts.StatusTooManyRequests, agent.ErrBusy.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	for ev := range events {
		if !publish(ev) {
			return // 客户端断开;Runner 的 goroutine 随 ctx 取消收敛
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

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
