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
	// resolve 按名字解析出实际要跑的配置(命中 profile 时用)。
	// 没命中 ⇒ 把名字当 acpx 内置 agent 名用(旧行为)。
	resolve func(name string) (Config, bool)
}

// NewDelegateRunner 构造委派执行器。
func NewDelegateRunner(cfg *Config) (*DelegateRunner, error) {
	cfg.Transport = TransportACPX
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DelegateRunner{cfg: *cfg}, nil
}

// WithResolver 装上 profile 解析器(按名字选外部 agent)。
func (d *DelegateRunner) WithResolver(f func(name string) (Config, bool)) *DelegateRunner {
	d.resolve = f
	return d
}

// Delegate 跑一次任务,返回 agent 的最终文本。
//
// agent 名字的解析顺序:
//  1. 命中已配置的 **profile 名** ⇒ 用那个 profile 的全部参数;
//  2. 没命中 ⇒ 当作 acpx 内置 agent 名(旧行为,保持兼容)。
func (d *DelegateRunner) Delegate(ctx context.Context, req delegate.Request) (string, error) {
	cfg := d.cfg
	if req.Agent != "" {
		if d.resolve != nil {
			if c, ok := d.resolve(req.Agent); ok {
				cfg = c
			} else {
				cfg.Agent = req.Agent
			}
		} else {
			cfg.Agent = req.Agent
		}
	}
	if req.Cwd != "" {
		cfg.Cwd = req.Cwd
	}
	return RunOnce(ctx, &cfg, req.Prompt)
}
