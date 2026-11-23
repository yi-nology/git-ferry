package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/oekit/concurrency/async"
	"github.com/oekit/logx"
	"github.com/oekit/pkg/ctxscope"
	"github.com/oekit/pkg/errors"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/yi-nology/goagent-kernel/checkpoint"
	"github.com/yi-nology/goagent-kernel/roundguard"
	"github.com/yi-nology/goagent-kernel/sideeffect"
	"github.com/yi-nology/goagent-runtime/agentloop"
	"github.com/yi-nology/goagent-runtime/chat"
	"github.com/yi-nology/goagent-runtime/pipeline"

	"github.com/yi-nology/git-ferry/internal/agent/agentstore"
	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// defaultMaxRounds 回合预算(与前身 eino adk MaxIterations=10 对齐)。
const defaultMaxRounds = 10

// logger 本包日志出口(经 logx 门面;调用时解析,跟随 main 装好的后端)。
var logger = logx.Delegating("ai")

// goagentEngine 内置后端(基于 goagent-runtime):可恢复回合循环 +
// 工具副作用闸 + SQLite 检查点持久化。
//
// 相比前身 eino adk 后端换来的确定性机制:
//   - 会话历史落检查点存储:进程重启后同一会话 ID 再 Run 即恢复,
//     历史要么完整要么没有这一段;
//   - 工具执行过 pipeline 副作用闸(幂等键 = 编排侧铸造的 toolCallID),
//     崩溃后恢复重放同一调用判 replayed,执行体不会被第二次调用;
//   - 回合预算走 kernel roundguard(缺省口径 = 强制收尾,模型体面结束)。
//
// 门面契约(engine.Engine)与 SSE 事件词表零变化;确认链路
// (PendingConfirm → tool_confirm → ExecuteConfirmed)原样保留。
type goagentEngine struct {
	reg *tools.Registry
	// pipe 工具施加管线(构造期从注册表投影,只建一次)。
	pipe *pipeline.Pipeline
	// checks 会话检查点存储(SQLite WAL,跨重启恢复锚点)。
	checks checkpoint.StoreCAS
	// effects 工具副作用闸(进程内;跨重启的恢复保护由检查点历史兜底)。
	effects sideeffect.Store
	closeFn func() error
	// model chat.Model 适配器(Stream 聚合并发 delta 事件)。
	model *goagentModel
	// budget 回合预算(kernel roundguard)。
	budget roundguard.Budget
	// memManifest 记忆摘要(SetMemory 热更新;影响新会话的系统提示)。
	memManifest string
	modelName   string
	// locks 每会话串行闸(chan 信号量):同会话并发 Run 排队,跨会话并行。
	// loop 的序号游标是进程内单调的,同一 Loop 不可并发 Run(loop 契约),
	// 故每 Run 新建 Loop 实例(共享 Store)+ 会话级排队,双保险。
	locks map[string]chan struct{}
	mu    sync.Mutex
}

// NewGoagentEngine 生产构造:OpenAI 兼容模型(BaseURL 可指内网 vLLM/Ollama)
// + SQLite 会话检查点库。dbPath 空串报错(匿名库 = 历史无处可寻,fail fast)。
func NewGoagentEngine(cfg *Config, apiKey string, svc tools.SyncService, dbPath string) (engine.Engine, error) {
	cm, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:      apiKey,
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		Temperature: ptrF32(cfg.Temperature),
		MaxTokens:   &cfg.MaxTokens,
		Timeout:     time.Duration(cfg.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, errors.Wrap(err, "构建模型失败")
	}
	return NewGoagentEngineWithModelAt(cfg, cm, tools.NewRegistry(svc), dbPath)
}

// NewGoagentEngineWithModel 测试构造:注入 eino 模型,进程内存储(无跨进程恢复)。
func NewGoagentEngineWithModel(cfg *Config, cm model.BaseModel[*schema.Message], reg *tools.Registry) (engine.Engine, error) {
	f := agentstore.NewMemory()
	return newGoagentEngine(cfg, cm, reg, f.Checks, f.Effects, nil)
}

// NewGoagentEngineWithModelAt 测试构造:注入 eino 模型 + SQLite 检查点库,
// 供跨重启恢复用例。dbPath 空串报错。
func NewGoagentEngineWithModelAt(cfg *Config, cm model.BaseModel[*schema.Message], reg *tools.Registry, dbPath string) (engine.Engine, error) {
	st, err := agentstore.Open(dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "打开 agent 会话库失败", "path", dbPath)
	}
	return newGoagentEngine(cfg, cm, reg, st.Checkpoints(), st.Effects(), func() error { return st.Close() })
}

// ptrF32 温度转指针(零值也显式下传,与 eino 版行为一致)。
func ptrF32(f float64) *float32 {
	v := float32(f)
	return &v
}

// newGoagentEngine 公共装配:管线投影 → 模型适配器 → 引擎。
func newGoagentEngine(cfg *Config, cm model.BaseModel[*schema.Message], reg *tools.Registry,
	checks checkpoint.StoreCAS, effects sideeffect.Store, closeFn func() error) (engine.Engine, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	proj, infos, err := projectTools(reg)
	if err != nil {
		return nil, err
	}
	pipe, err := pipeline.New(effects, 30*time.Second, proj)
	if err != nil {
		return nil, errors.Wrap(err, "构建工具施加管线失败")
	}
	return &goagentEngine{
		reg:         reg,
		pipe:        pipe,
		checks:      checks,
		effects:     effects,
		closeFn:     closeFn,
		model:       newGoagentModel(cm, infos),
		budget:      roundguard.Budget{MaxRounds: defaultMaxRounds},
		memManifest: cfg.MemoryManifest,
		modelName:   cfg.Model,
		locks:       map[string]chan struct{}{},
	}, nil
}

// projectTools 把工具注册表投影成 pipeline.Tool 列表 + eino ToolInfo 索引。
//
// 危险标注单源派生 Safe:
//   - 非危险 → Safe=true(幂等声明:读路径重跑无害,错误 = 明确失败);
//   - 危险 → 不声明(按最弱处理:真实动作报错 → 结果未知,禁止盲目重试)。
//
// 依据:dangerGuard 的确认/拒绝路径全部返回 (JSON, nil),只有真实动作
// 才返回 error,所以这套映射在确认链路下语义自洽。
func projectTools(reg *tools.Registry) ([]pipeline.Tool, map[string]*schema.ToolInfo, error) {
	names := reg.Names()
	out := make([]pipeline.Tool, 0, len(names))
	infos := make(map[string]*schema.ToolInfo, len(names))
	for _, name := range names {
		tl := reg.ByName(name)
		if tl == nil {
			continue
		}
		info, err := tl.Info(context.Background())
		if err != nil {
			return nil, nil, errors.Wrapf(err, "读取工具 %q 元信息失败", name)
		}
		infos[name] = info
		out = append(out, pipeline.Tool{
			Name:        name,
			Description: info.Desc,
			Safe:        !reg.IsDanger(name),
			// Parameters 留空:模型面由适配器按名回查 ToolInfo(免 JSON 往返)。
			Parameters: nil,
			Handler:    toolBridge(name, reg),
		})
	}
	return out, infos, nil
}

// toolBridge 工具执行桥:pipeline.Handler → 注册表 InvokableRun。
// 事件语义与 eino 装饰器(decorator.go)同源:tool_start → 执行 →
// 大输出落盘 → confirmation_required 升级 tool_confirm → tool_end。
// ctx 里的 sink 由引擎 Run 装配。
func toolBridge(name string, reg *tools.Registry) pipeline.Handler {
	// 复用 eino 装饰器(DecorateTool):tool_start/tool_end/tool_confirm、
	// 大输出落盘、confirmation_required 升级等事件语义单源,
	// 与生产路径完全一致(decorator_test 钉住的正是这套)。
	decorated := DecorateTool(name, reg.ByName(name))
	return func(ctx context.Context, args []byte) (string, error) {
		out, err := decorated.InvokableRun(ctx, string(args))
		if err != nil {
			return "", err
		}
		return out, nil
	}
}

// ============================================================
// chat.Model 适配器:Stream 聚合出 delta 事件
// ============================================================

// goagentModel 把 eino OpenAI 兼容模型包成 goagent chat.Model。
//
// 流式不回退的关键:chat.Model 契约是**单次返回完整响应**,但适配器内部
// 走 eino Stream —— 逐 chunk 经 ctx sink 发 SSE delta 事件(流式体验不变),
// 同时聚合出完整 ChatResponse 交给 loop。流式是模型层关注点,不进 loop。
type goagentModel struct {
	cm model.BaseModel[*schema.Message]
	// infos 工具名 → eino ToolInfo(模型声明工具面用,免 JSON 往返)。
	infos map[string]*schema.ToolInfo
}

// usageKey ctx 携带的 usage 累计器(engine.Run 装配,适配器写入)。
var usageKey = ctxscope.NewKey[*engine.Usage]("git-ferry/agent.Usage")

// newGoagentModel 构造适配器。
func newGoagentModel(cm model.BaseModel[*schema.Message], infos map[string]*schema.ToolInfo) *goagentModel {
	return &goagentModel{cm: cm, infos: infos}
}

// Chat 一次模型调用:转消息 → Stream → 逐 chunk 发 delta → 聚合完整响应。
func (m *goagentModel) Chat(ctx context.Context, req chat.ChatRequest) (chat.ChatResponse, error) {
	msgs := einoMessages(req.Messages)
	var opts []model.Option
	if len(req.Tools) > 0 {
		opts = append(opts, model.WithTools(einoToolInfos(req.Tools, m.infos)))
	}
	sr, err := m.cm.Stream(ctx, msgs, opts...)
	if err != nil {
		return chat.ChatResponse{}, errors.Wrap(err, "模型流调用失败")
	}
	var (
		content strings.Builder
		usage   *engine.Usage
	)
	// 聚合器:流式 chunk 的工具调用按 Index 归并;内容与 usage 逐 chunk 累计。
	type callAgg struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*callAgg{}
	order := []int{}
	sink := sinkFromCtx(ctx)
	for {
		chunk, cerr := sr.Recv()
		if errors.Is(cerr, io.EOF) {
			break
		}
		if cerr != nil {
			return chat.ChatResponse{}, errors.Wrap(cerr, "模型流接收失败")
		}
		if chunk == nil {
			continue
		}
		if chunk.Content != "" {
			content.WriteString(chunk.Content)
			sink(Event{Type: engine.EventDelta, Content: chunk.Content})
		}
		for i := range chunk.ToolCalls {
			tc := chunk.ToolCalls[i]
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			agg, ok := calls[idx]
			if !ok {
				agg = &callAgg{}
				calls[idx] = agg
				order = append(order, idx)
			}
			if tc.ID != "" {
				agg.id = tc.ID
			}
			if tc.Function.Name != "" {
				agg.name = tc.Function.Name
			}
			agg.args.WriteString(tc.Function.Arguments)
		}
		if usage == nil {
			usage = usageFromCtx(ctx)
		}
		if usage != nil {
			accumulateUsage(usage, chunk)
		}
	}
	out := chat.ChatResponse{Content: content.String()}
	for _, idx := range order {
		agg := calls[idx]
		if agg.name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, chat.ToolCall{ID: agg.id, Name: agg.name, Arguments: agg.args.String()})
	}
	return out, nil
}

// usageFromCtx 取 usage 累计器(缺省 nil = 不统计)。
func usageFromCtx(ctx context.Context) *engine.Usage {
	u, _ := ctxscope.From(ctx, usageKey)
	return u
}

// einoMessages goagent 消息 → eino 消息。
func einoMessages(in []chat.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(in))
	for _, m := range in {
		role := schema.User
		switch m.Role {
		case chat.RoleSystem:
			role = schema.System
		case chat.RoleAssistant:
			role = schema.Assistant
		case chat.RoleTool:
			role = schema.Tool
		}
		sm := &schema.Message{Role: role, Content: m.Content}
		if m.Role == chat.RoleAssistant {
			for _, c := range m.ToolCalls {
				sm.ToolCalls = append(sm.ToolCalls, schema.ToolCall{
					ID:   c.ID,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      c.Name,
						Arguments: c.Arguments,
					},
				})
			}
		}
		if m.Role == chat.RoleTool {
			sm.ToolCallID = m.ToolCallID
		}
		out = append(out, sm)
	}
	return out
}

// einoToolInfos 工具声明 → eino ToolInfo:按名回查注册表 projectTools 索引;
// 未命中(理论不可达,双源都是注册表)时退化为纯名字声明。
func einoToolInfos(specs []chat.ToolSpec, infos map[string]*schema.ToolInfo) []*schema.ToolInfo {
	out := make([]*schema.ToolInfo, 0, len(specs))
	for _, s := range specs {
		if info, ok := infos[s.Name]; ok {
			out = append(out, info)
			continue
		}
		out = append(out, &schema.ToolInfo{Name: s.Name, Desc: s.Description})
	}
	return out
}

// ============================================================
// 引擎:engine.Engine 实现
// ============================================================

// ID 后端标识(与 eino 版一致:internal)。
func (e *goagentEngine) ID() string { return "internal" }

// Label 展示名。
func (e *goagentEngine) Label() string { return "内置助手 (goagent-runtime)" }

// ModelName 返回模型名(状态接口展示)。
func (e *goagentEngine) ModelName() string { return e.modelName }

// Close 收尾:关 SQLite 检查点库。
func (e *goagentEngine) Close() error {
	if e.closeFn != nil {
		return e.closeFn()
	}
	return nil
}

// SetPolicy 注入工具权限策略。
func (e *goagentEngine) SetPolicy(p *tools.Policy) {
	if e.reg != nil {
		e.reg.SetPolicy(p)
	}
}

// SetDelegator 注入外部 coding agent 委派执行器(可选)。
func (e *goagentEngine) SetDelegator(d delegate.Runner) {
	if e.reg != nil {
		e.reg.SetDelegator(d)
	}
}

// DelegateStatus 查委派任务状态(外部 agent 委派是异步的)。
func (e *goagentEngine) DelegateStatus(taskID string) (map[string]any, bool) {
	if e.reg == nil {
		return nil, false
	}
	return e.reg.DelegateStatus(taskID)
}

// HasDelegator 是否装配了委派执行器(诊断用)。
func (e *goagentEngine) HasDelegator() bool {
	return e.reg != nil && e.reg.HasDelegator()
}

// SetMemory 注入记忆库与 prompt 摘要(可选)。
// 与 eino 版的差异:系统提示按**每次 Run** 取 BuildPrompt(memManifest),
// 所以这里的热更新对新会话立即生效(eino 版是构造期固化,不生效)。
func (e *goagentEngine) SetMemory(mem tools.MemStore, manifest string) {
	if e.reg != nil {
		e.reg.SetMemoryStore(mem)
	}
	e.memManifest = manifest
}

// lockFor 取(或建)会话串行闸。
func (e *goagentEngine) lockFor(id string) chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	lk, ok := e.locks[id]
	if !ok {
		lk = make(chan struct{}, 1)
		e.locks[id] = lk
	}
	return lk
}

// Run 执行一轮对话:goagent loop 续跑会话 → 事件推入返回的 chan。
//
// 与 eino 版同构的编排:ctx 取消即断流;事件经 ctx sink 分发(工具桥与
// 模型适配器都读它);每会话串行闸排队;sessionID = 检查点 RunID。
func (e *goagentEngine) Run(ctx context.Context, sess engine.Session, in engine.Input) (<-chan engine.Event, context.CancelFunc, error) {
	// 独立可取消:消费方断开时 cancel,模型流与事件发送随之终止。
	ctx, cancel := context.WithCancel(ctx)

	out := make(chan engine.Event, 64)
	// emit 是唯一事件出口:select 双路保证消费方消失后 goroutine 必然收敛。
	emit := func(ev engine.Event) {
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	}
	// delta 累计(display 历史;loop 的协议历史在检查点存储里)。
	var full strings.Builder
	ctx = withEventSink(ctx, func(ev engine.Event) {
		if ev.Type == engine.EventDelta {
			full.WriteString(ev.Content)
		}
		emit(ev)
	})
	ctx = ctxscope.With(ctx, usageKey, &engine.Usage{})
	// 工具层要拿到会话能力(危险工具确认、权限档)。
	ctx = tools.WithScope(ctx, sess)
	lock := e.lockFor(sess.SessionID())

	async.GoSafe(func() {
		defer close(out)
		defer cancel()
		// panic 要作为 error 事件推给客户端,不能只靠 GoSafe 全局日志。
		defer func() {
			if rec := recover(); rec != nil {
				emit(engine.Event{Type: engine.EventError, Content: fmt.Sprintf("内部错误: %v", rec)})
				logger.Error("ai agent panic", "panic", rec, "session", sess.SessionID())
			}
		}()

		// 每会话串行:同会话并发 Run 排队(ctx 取消即弃权,不占位)。
		select {
		case lock <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-lock }()

		loop, err := agentloop.New(agentloop.Config{
			Model:  e.model,
			Tools:  e.pipe,
			Store:  e.checks,
			Budget: e.budget,
			System: BuildPrompt(e.memManifest),
		})
		if err != nil {
			emit(engine.Event{Type: engine.EventError, Content: errText(err)})
			return
		}
		res, err := loop.Run(ctx, sess.SessionID(), in.Message)
		if err != nil {
			// ctx 取消上抛为错误:快照已持久化,下次同 sessionID 再 Run 即恢复。
			emit(engine.Event{Type: engine.EventError, Content: errText(err)})
			return
		}
		usage := usageFromCtx(ctx)
		if strings.TrimSpace(full.String()) != "" {
			sess.Append("assistant", full.String())
		} else if res.Final != "" {
			sess.Append("assistant", res.Final)
		}
		done := engine.Event{Type: engine.EventDone, Usage: usage}
		if len(res.Uncertain) > 0 {
			done.Content = fmt.Sprintf("警告: %d 个工具结果未知(需对账后才能重试)", len(res.Uncertain))
		}
		emit(done)
	})
	return out, cancel, nil
}

// ExecuteConfirmed 确认后直达执行:校验令牌 → 标记放行 → 直调工具 →
// 结果写入会话(不经过模型)。返回工具输出的 JSON 文本。
func (e *goagentEngine) ExecuteConfirmed(ctx context.Context, sess engine.Session, toolName, token string) (string, error) {
	tl := e.reg.ByName(toolName)
	if tl == nil {
		return "", errors.Newf("未知工具 %q", toolName)
	}
	args, err := sess.ConsumePending(toolName, token)
	if err != nil {
		return "", err
	}
	sess.MarkConsumed(toolName, args)
	out, err := tl.InvokableRun(tools.WithScope(ctx, sess), args)
	if err != nil {
		out = failJSON(err)
	}
	sess.Append("assistant", fmt.Sprintf("(已执行 %s)%s", toolName, out))
	return out, nil
}

// errText 错误 → 用户可读文本(ctx 取消 = 已取消)。
func errText(err error) string {
	if errors.Is(err, context.Canceled) {
		return "已取消"
	}
	return fmt.Sprintf("模型调用失败: %v", err)
}

// failJSON 确认执行失败编码为 JSON。
func failJSON(err error) string {
	b, _ := json.Marshal(map[string]any{"status": "failed", "error": err.Error()})
	return string(b)
}

// accumulateUsage 累计 eino chunk 的 token 用量。
func accumulateUsage(u *engine.Usage, msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	u.InputTokens += msg.ResponseMeta.Usage.PromptTokens
	u.OutputTokens += msg.ResponseMeta.Usage.CompletionTokens
}
