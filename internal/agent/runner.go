package agent

import (
	"context"
	"errors"
	"time"

	"github.com/oekit/concurrency/async"
	"github.com/oekit/concurrency/syncx"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// Runner AI 助手门面:并发闸门 + 会话存储 + 事件转发。
//
// **执行交给后端引擎**(engine.Engine):
//
//   - 内置后端(eino):进程内调 OpenAI 兼容模型,41 个领域工具在本进程直调 core;
//   - 外部后端(acpx/acp):把回合交给外部 coding agent 子进程,
//     工具在 agent 侧执行(经 git-ferry 自己的 /mcp)。
//
// 门面只做三件**与后端无关**的事:并发闸门、会话记账、事件转发。
// 它不知道工具长什么样、也不知道是谁在执行 —— 这正是可插拔的前提。
type Runner struct {
	eng      engine.Engine
	sessions *SessionStore
	// stopCtx 用来停掉会话回收 goroutine(Close 时 cancel)。
	stopCtx context.Context
	stop    context.CancelFunc
	// gate 是全局在飞闸门(并发上限 = maxConcurrentChats)。
	// 用 oekit concurrency/syncx.Gate 的 TryAcquire(满额即拒,不排队):
	// 语义与迁移前的 chan 缓冲 + select/default 完全一致,
	// 但 release 幂等(重复调用只归还一次),不会重复归还名额。
	gate *syncx.Gate
}

// NewRunner 构造门面。maxConcurrentChats <= 0 时用默认值。
func NewRunner(eng engine.Engine, maxConcurrentChats int) *Runner {
	if maxConcurrentChats <= 0 {
		maxConcurrentChats = defaultMaxConcurrentChats
	}
	stopCtx, stop := context.WithCancel(context.Background())
	r := &Runner{
		eng:      eng,
		sessions: NewSessionStore(sessionTTL, sessionMaxRounds),
		gate:     syncx.NewGate(maxConcurrentChats),
		stopCtx:  stopCtx,
		stop:     stop,
	}
	// 进程级生命周期:回收再无人访问的过期会话,防内存慢涨
	r.sessions.StartJanitor(stopCtx, 5*time.Minute)
	return r
}

// Close 释放后端资源(关掉外部 agent 子进程)并停掉回收 goroutine。
//
// **热重建时必须调**:外部后端每个会话对应一个**长驻子进程**
// (原生 ACP transport),换后端不关旧的话,机器上会留下一堆孤儿 agent 进程。
func (r *Runner) Close() error {
	if r.stop != nil {
		r.stop()
	}
	if r.eng != nil {
		return r.eng.Close()
	}
	return nil
}

// Sessions 暴露会话存储(handler 建会话用)。
func (r *Runner) Sessions() *SessionStore { return r.sessions }

// ModelName 返回模型名(状态接口展示)。
func (r *Runner) ModelName() string { return r.eng.ModelName() }

// BackendID 返回后端标识(internal|acpx|acp)。
func (r *Runner) BackendID() string { return r.eng.ID() }

// BackendLabel 返回后端展示名。
func (r *Runner) BackendLabel() string { return r.eng.Label() }

// Ready 后端的静态就位性(快,不启动进程)。
//
// 内置后端没有这个概念(它的就位性 = 模型端点可达,由「测试连接」验);
// 外部后端答的是「装没装」。
func (r *Runner) Ready() (bool, string) {
	if rd, ok := r.eng.(engine.Readiness); ok {
		return rd.Ready()
	}
	return true, ""
}

// SetMemory 注入记忆库与 prompt 摘要(可选)。
// SetPolicy 注入工具权限策略。
//
// 两者都是**可选能力**:外部后端没有本地工具表,注入即无意义,
// 此时如实忽略(而不是假装生效)。
func (r *Runner) SetPolicy(p *tools.Policy) {
	if b, ok := r.eng.(engine.PolicyBinder); ok {
		b.SetPolicy(p)
	}
}

// SetDelegator 注入外部 coding agent 委派执行器(可选)。
//
// 只有内置后端用得上(委派是内置助手的工具能力)。
func (r *Runner) SetDelegator(d delegate.Runner) {
	if b, ok := r.eng.(engine.DelegatorBinder); ok {
		b.SetDelegator(d)
	}
}

func (r *Runner) SetMemory(mem tools.MemStore, manifest string) {
	if b, ok := r.eng.(engine.MemoryBinder); ok {
		b.SetMemory(mem, manifest)
	}
}

// Run 执行一轮对话:记账 → 交给后端 → 转发事件。
// chan 关闭即本轮结束;error 事件后 chan 也会关闭。
// 返回的 cancel 供消费方提前终止(如客户端断开):取消后所有事件发送与
// 后端执行立即收敛,goroutine 退出并释放并发信号量,不再产生 token 消耗。
func (r *Runner) Run(ctx context.Context, sess *Session, userMsg string) (<-chan Event, context.CancelFunc, error) {
	// 并发闸门:满额立即拒(ErrBusy),不排队——与迁移前的 chan + select/default 语义一致
	release, ok := r.gate.TryAcquire()
	if !ok {
		return nil, nil, ErrBusy
	}

	// 独立可取消:消费方断开时 cancel,后端执行与事件发送随之终止
	ctx, cancel := context.WithCancel(ctx)

	// 会话记账:**与后端无关**,两条链路都走这里。
	// 用户消息先入账,后端据此拼装上下文;助手回复由后端在回合结束时追加。
	r.sessions.Append(sess, "user", userMsg)
	// 上下文压缩:超过阈值丢弃远古细节
	if histMax := 40; histMax > 0 {
		r.sessions.Compact(sess, histMax)
	}

	ch, engCancel, err := r.eng.Run(ctx, sess, engine.Input{Message: userMsg})
	if err != nil {
		release()
		cancel()
		return nil, nil, err
	}

	out := make(chan Event, 64)
	// start 先发:前端据此建立本轮上下文(会话 id)。
	// 缓冲为空,此处同步发送不会阻塞。
	out <- Event{Type: engine.EventStart, SessionID: sess.ID}

	async.GoSafe(func() {
		defer close(out)
		defer release()
		defer engCancel()
		for ev := range ch {
			// 双路 select:消费方消失后本 goroutine 必然收敛,
			// 不会因 chan 无消费者而永久阻塞(否则信号量永不释放,AI 功能自锁)。
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	})
	return out, cancel, nil
}

// ExecuteConfirmed 确认后直达执行:校验令牌 → 标记放行 → 直调工具 →
// 结果写入会话(不经过模型)。返回工具输出的 JSON 文本。
//
// 这是**内置后端**的能力(工具在本进程内)。外部后端的工具在 agent 侧执行,
// 确认语义是 ACP 的 permission 链路,走 RespondPermission。
func (r *Runner) ExecuteConfirmed(ctx context.Context, sess *Session, toolName, token string) (string, error) {
	c, ok := r.eng.(engine.Confirmer)
	if !ok {
		return "", errors.New("当前 AI 后端不支持确认直达执行(外部 coding agent 走授权回传链路)")
	}
	return c.ExecuteConfirmed(ctx, sess, toolName, token)
}

// DelegateStatus 查「外部 agent 委派任务」的状态(前端轮询用)。
//
// 委派是**异步**的:确认后立刻返回任务 ID,结果稍后才有。
func (r *Runner) DelegateStatus(taskID string) (map[string]any, bool) {
	if ds, ok := r.eng.(engine.DelegateStatus); ok {
		return ds.DelegateStatus(taskID)
	}
	return nil, false
}

// RespondPermission 把用户选定的授权项回传给外部后端(ACP permission)。
// 内置后端不支持 —— 它没有 permission 请求。
func (r *Runner) RespondPermission(ctx context.Context, sessionID, requestID, optionID string) error {
	p, ok := r.eng.(engine.PermissionResponder)
	if !ok {
		return errors.New("当前 AI 后端不支持授权回传")
	}
	return p.RespondPermission(ctx, sessionID, requestID, optionID)
}
