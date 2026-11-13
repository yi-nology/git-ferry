package acp

import (
	"context"
	"fmt"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// RunOnceStdio 用原生 ACP stdio 跑一次性任务(委派场景)。
//
// 与 RunOnce(acpx exec)的分工:
//   - acpx exec:另起一个 acpx 进程跑一句话,无会话、无记忆;
//   - 本函数:真的起一个 ACP agent 进程 → initialize → session/new →
//     session/prompt → 收结果 → 关进程。
//
// **为什么要支持它**:具名 profile 里可以是 stdio 形态的 agent(如 "mcode acp")。
// 委派按名字选 profile 时可能选到它 —— 若硬塞给 acpx(--agent "<command>")
// 就是把「命令」当成 acpx 的 agent 名,必然跑不起来。
//
// **授权**:一次性执行弹不出授权卡 ⇒ ask 档在这里没有对应物,直接拒绝
// (由调用方把 ask 映射成 approve_reads)。不静默降级。
func RunOnceStdio(ctx context.Context, cfg *Config, prompt string) (string, error) {
	c := *cfg
	c.Transport = TransportStdio
	if c.Permission == PermissionAsk {
		return "", fmt.Errorf("stdio 一次性执行是非交互的,无法逐次弹授权卡:" +
			"授权档请选 approve_reads | approve_all | deny")
	}
	eng, err := NewEngine(&c)
	if err != nil {
		return "", err
	}
	defer func() { _ = eng.Close() }()

	proc, err := eng.start(ctx, "delegate-once")
	if err != nil {
		return "", err
	}
	defer proc.close()
	proc.beginTurn()
	defer proc.endTurn()

	// 事件流在委派场景不外发(只要最终文本);授权由 autoDecide 按档自动决策。
	return eng.prompt(ctx, proc, prompt, func(engine.Event) {})
}
