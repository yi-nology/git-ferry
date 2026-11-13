package acp

import (
	"context"
	"fmt"

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
//
// **传输方式由配置决定**(acpx 或 stdio),不再一律强制 acpx ——
// 具名 profile 里可能有 stdio 形态的 agent,强制 acpx 会把它的命令
// 当成 acpx 的 agent 名,必然跑不起来。
func NewDelegateRunner(cfg *Config) (*DelegateRunner, error) {
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
//     stdio 传输下没有「内置 agent 名」这回事 ⇒ 直接报错,
//     不拿一个名字去当命令跑(那必然失败,还看不出真实原因)。
func (d *DelegateRunner) Delegate(ctx context.Context, req delegate.Request) (string, error) {
	cfg := d.cfg
	if req.Agent != "" {
		resolved := false
		if d.resolve != nil {
			if c, ok := d.resolve(req.Agent); ok {
				cfg = c
				resolved = true
			}
		}
		if !resolved {
			// acpx:没命中 profile ⇒ 当作 acpx 内置 agent 名用(旧行为)。
			// stdio:没有「内置 agent 名」这回事 ⇒ 拿名字当命令跑必然失败,
			// 而且看不出真实原因 ⇒ 直接报错(不静默跑成默认 agent)。
			if cfg.Transport == TransportStdio {
				return "", fmt.Errorf("未找到名为 %q 的外部 agent profile;"+
					"stdio 传输的 agent 要用 command 指定,不能只给名字", req.Agent)
			}
			cfg.Agent = req.Agent
		}
	}
	if req.Cwd != "" {
		cfg.Cwd = req.Cwd
	}
	switch cfg.Transport {
	case TransportStdio:
		return RunOnceStdio(ctx, &cfg, req.Prompt)
	default:
		return RunOnce(ctx, &cfg, req.Prompt)
	}
}
