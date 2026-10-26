package acp

import (
	"context"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
)

// DelegateRunner 把任务委派给外部 coding agent(一次性执行)。
//
// 与「后端整体切换」的区别:
//   - 后端切换:整个会话交给外部 agent,界面直接和它对话;
//   - 委派:内置助手在需要时**派一段活**出去,结果回到助手的上下文里。
//
// 走 acpx 的 exec 形态:无会话、无记忆、跑完即走 —— 委派要的就是这种确定性。
type DelegateRunner struct {
	cfg Config
}

// NewDelegateRunner 构造委派执行器。
func NewDelegateRunner(cfg *Config) (*DelegateRunner, error) {
	cfg.Transport = TransportACPX
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DelegateRunner{cfg: *cfg}, nil
}

// Delegate 跑一次任务,返回 agent 的最终文本。
func (d *DelegateRunner) Delegate(ctx context.Context, req delegate.Request) (string, error) {
	cfg := d.cfg
	if req.Agent != "" {
		cfg.Agent = req.Agent
	}
	if req.Cwd != "" {
		cfg.Cwd = req.Cwd
	}
	return RunOnce(ctx, &cfg, req.Prompt)
}
