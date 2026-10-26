package tools

import (
	"context"
	"encoding/json"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
)

// SetDelegator 注入外部 coding agent 的委派执行器(可选)。
//
// 未注入时 ai_code_delegate 工具仍会注册,但调用时如实返回「未配置」。
func (r *Registry) SetDelegator(d delegate.Runner) { r.delegator = d }

type delegateInput struct {
	Prompt string `json:"prompt" jsonschema:"交给外部 coding agent 的任务描述,要自包含(agent 看不到我们的上下文)"`
	Agent  string `json:"agent,omitempty" jsonschema:"覆盖默认 agent 名(如 mcode/codex/claude)"`
	Cwd    string `json:"cwd,omitempty" jsonschema:"覆盖默认工作目录"`
}

// delegateIn 把一段编码任务委派给外部 coding agent。
//
// **标为危险**:外部 agent 是**跑在服务端主机上的子进程**,拥有该用户的
// 文件与网络权限 ⇒ 必须走确认链路(用户要明确知道要把什么任务交出去)。
//
// 与「后端整体切换」的区别:那条路是**整个会话**交给外部 agent;
// 这条路是内置助手在需要时**派一段活**出去,结果回到助手的上下文里。
func (r *Registry) delegateIn(ctx context.Context, in delegateInput) (string, error) {
	return r.dangerGuard(ctx, "ai_code_delegate", marshalJSON(in),
		func(ctx context.Context, args string) (string, error) {
			var in delegateInput
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return errJSON("参数解析失败", err), nil
			}
			if in.Prompt == "" {
				return errJSON("prompt 不能为空", nil), nil
			}
			if r.delegator == nil {
				return errJSON("外部 coding agent 未配置(请在 AI 设置里填 external.agent / external.command)", nil), nil
			}
			out, err := r.delegator.Delegate(ctx, delegate.Request{
				Prompt: in.Prompt,
				Agent:  in.Agent,
				Cwd:    in.Cwd,
			})
			if err != nil {
				return errJSON("委派外部 agent 失败: "+err.Error(), err), nil
			}
			return marshalJSON(map[string]any{"status": "ok", "result": out}), nil
		})
}
