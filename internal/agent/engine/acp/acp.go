package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oekit/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oekit/concurrency/async"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// Engine 原生 ACP 传输:自己当 client,直接对 agent 子进程说 JSON-RPC。
//
// 这是**功能完整**的外部后端:
//   - 能拿到 session/request_permission ⇒ 外部 agent 的写操作可以接进
//     git-ferry 的确认 UI(而不是由外部 CLI 静默决定);
//   - 能把 git-ferry 自己的 /mcp 挂给 agent ⇒ 外部 agent 直接获得
//     41 个领域工具,不必重写。
//
// 代价:每个 git-ferry 会话对应一个**长驻 agent 进程**,要管生命周期
// (空闲回收、崩溃重启、进程数上限)。
type Engine struct {
	cfg Config

	mu    sync.Mutex
	procs map[string]*sessionProc
	perms *permRegistry
	fs    *fsHandler
	terms *terminalHandler

	stopOnce sync.Once
	stop     chan struct{}

	// audit 外部 agent 行为的审计出口(可选)。
	audit AuditFunc
}

// NewEngine 构造原生 ACP 后端。
func NewEngine(cfg *Config) (*Engine, error) {
	cfg.Transport = TransportStdio
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(cfg.Cwd)
	if err != nil {
		return nil, errors.Wrap(err, "解析工作目录失败")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, errors.Wrap(err, "创建工作目录失败")
	}
	cfg.Cwd = root
	// **一个沙箱,两条能力共用**:fs/* 与 terminal/* 的边界判定必须是同一份
	// (各写一套 ⇒ 迟早漂移:terminal 的 cwd 曾经直接用 agent 给的)。
	sb, err := newSandbox(root)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		cfg:   *cfg,
		procs: make(map[string]*sessionProc),
		perms: newPermRegistry(),
		fs:    &fsHandler{sandbox: sb},
		terms: newTerminalHandlerWith(sb),
		stop:  make(chan struct{}),
	}
	e.startJanitor()
	return e, nil
}

// ID 后端标识。
func (e *Engine) ID() string { return TransportStdio }

// Label 展示名。
func (e *Engine) Label() string {
	name := e.cfg.Command
	if e.cfg.Agent != "" {
		name = e.cfg.Agent
	}
	return "外部 coding agent (" + name + " / ACP)"
}

// ModelName 展示用 agent 名。
func (e *Engine) ModelName() string {
	if e.cfg.Agent != "" {
		return e.cfg.Agent
	}
	return e.cfg.Command
}

// SetAudit 注入审计出口(可选)。
//
// 外部 agent 的工具调用在**它自己的进程里**发生,git-ferry 既有的操作审计
// 看不见 ⇒ 这里补一条,让「agent 在本机做了什么」可查。
func (e *Engine) SetAudit(f AuditFunc) { e.audit = f }

// auditCall 记一条审计(失败只告警,不影响主流程)。
func (e *Engine) auditCall(action, tool, detail string) {
	if e.audit == nil {
		return
	}
	e.audit(context.Background(), action, tool, detail)
}

// Ready 静态就位性:命令在不在 PATH 上(快,不启动进程)。
func (e *Engine) Ready() (bool, string) { return e.cfg.Ready() }

// Close 关闭所有 agent 进程。
func (e *Engine) Close() error {
	e.stopOnce.Do(func() { close(e.stop) })
	e.mu.Lock()
	procs := make([]*sessionProc, 0, len(e.procs))
	for _, p := range e.procs {
		procs = append(procs, p)
	}
	e.procs = make(map[string]*sessionProc)
	e.mu.Unlock()
	for _, p := range procs {
		p.close()
	}
	return nil
}

// Run 执行一轮对话。
func (e *Engine) Run(ctx context.Context, sess engine.Session, in engine.Input) (<-chan engine.Event, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan engine.Event, 64)

	emit := func(ev engine.Event) {
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	}

	proc, err := e.procFor(ctx, sess.SessionID())
	if err != nil {
		cancel()
		return nil, nil, err
	}
	proc.beginTurn()
	defer proc.endTurn()

	async.GoSafe(func() {
		defer close(out)
		defer cancel()
		text, err := e.prompt(ctx, proc, in.Message, emit)
		if err != nil {
			emit(engine.Event{Type: engine.EventError, Content: err.Error()})
			return
		}
		sess.Append("assistant", text)
		emit(engine.Event{Type: engine.EventDone})
	})
	return out, cancel, nil
}

// prompt 发一轮提示并收集事件。
func (e *Engine) prompt(ctx context.Context, proc *sessionProc, message string, emit func(engine.Event)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	// 同一 ACP 会话上的回合**串行**:并发会互相覆盖 sink,而且协议上也不允许。
	proc.turnMu.Lock()
	defer proc.turnMu.Unlock()

	// 本轮的 sink:session/update 通知由 reader goroutine 派发到这里。
	//
	// 回合文本在事件流里已经有了,这里顺带聚一份(供会话记账)。
	var text strings.Builder
	proc.setSink(func(ev engine.Event) {
		if ev.Type == engine.EventDelta {
			text.WriteString(ev.Content)
		}
		emit(ev)
	})
	defer proc.setSink(nil)

	// **把外部 agent 的会话 ID 透出去**:外部 agent 与本服务可能共用同一个
	// 会话库(如 MiniMax Code 的 CLI 与桌面版)—— 用户要能据此在那个界面上
	// 找到这次会话(看记录、接着聊),否则这个能力等于没有入口。
	proc.sinkEvent(&engine.Event{
		Type:           engine.EventAgentSession,
		AgentSessionID: proc.sessionID,
		AgentLabel:     proc.agentName,
	})

	// 取消时通知 agent 停下(session/cancel 是通知,无需响应)。
	// finished 用于区分「正常结束」与「被取消」—— 否则回合结束时的
	// defer cancel 会误发一次 cancel,把**下一轮**也取消掉。
	finished := make(chan struct{})
	defer close(finished)
	async.GoSafe(func() {
		select {
		case <-ctx.Done():
			_ = proc.client.notify("session/cancel", map[string]any{"sessionId": proc.sessionID})
		case <-finished:
		}
	})

	params := map[string]any{
		"sessionId": proc.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": message}},
	}
	var res struct {
		StopReason string `json:"stopReason"`
	}
	if err := proc.client.call(ctx, "session/prompt", params, &res); err != nil {
		return text.String(), errors.Wrap(err, "ACP 回合失败")
	}
	return text.String(), nil
}

// RespondPermission 把用户选定的授权项回传给 agent。
func (e *Engine) RespondPermission(ctx context.Context, sessionID, requestID, optionID string) error {
	return e.perms.respond(requestID, optionID)
}

// procFor 取(或建)该 git-ferry 会话对应的 agent 进程与 ACP 会话。
func (e *Engine) procFor(ctx context.Context, key string) (*sessionProc, error) {
	e.mu.Lock()
	p, ok := e.procs[key]
	if ok {
		p.touch()
		e.mu.Unlock()
		return p, nil
	}
	// 进程数上限:先回收最久未用的。
	if len(e.procs) >= e.cfg.MaxProcs {
		e.evictOldest()
	}
	e.mu.Unlock()

	p, err := e.start(ctx, key)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	// 并发下可能已有别人建好了同一个会话的进程 —— 留一个,关掉多余的。
	if old, dup := e.procs[key]; dup {
		e.mu.Unlock()
		p.close()
		old.touch()
		return old, nil
	}
	e.procs[key] = p
	e.mu.Unlock()
	return p, nil
}

// evictOldest 回收最久未用的进程(调用方持锁)。
//
// **跳过正在跑回合的进程** —— 回收它会把在飞的回合打断。
// 全都在忙时宁可不回收(进程数短暂超限),也不要打断别人的活。
func (e *Engine) evictOldest() {
	var oldestKey string
	var oldest *sessionProc
	for k, p := range e.procs {
		if p.busy() {
			continue
		}
		if oldest == nil || p.lastUsed().Before(oldest.lastUsed()) {
			oldestKey, oldest = k, p
		}
	}
	if oldest == nil {
		return
	}
	delete(e.procs, oldestKey)
	// **回收放在可恢复的 goroutine 里**:后台 goroutine 的 panic
	// 没有人会接 ⇒ 直接带走整个服务。关闭进程只是清理,不该有全局杀伤力。
	async.GoSafe(func() { oldest.close() })
}

// start 启动 agent 进程并完成 initialize + session/new。
func (e *Engine) start(ctx context.Context, key string) (*sessionProc, error) {
	fields := splitCommand(e.cfg.Command)
	if len(fields) == 0 {
		return nil, errors.New("command 不能为空")
	}
	pctx, cancel := context.WithCancel(context.Background())
	// 命令由管理员在配置里指定(不是外部请求输入);工作目录即沙箱边界。
	cmd := exec.CommandContext(pctx, fields[0], fields[1:]...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = e.cfg.Cwd
	// **环境要净化**:默认继承父进程 ⇒ agent 能读到 ENCRYPTION_KEY 与 admin key,
	// 绕开 MCP 的只读边界。见 env.go 的说明。
	cmd.Env = scrubbedEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "agent stdin 管道失败")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "agent stdout 管道失败")
	}
	stderr := newRingBuffer(16 * 1024)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, errors.Wrap(err, fmt.Sprintf("启动 agent 进程失败(%s)", e.cfg.Command))
	}

	proc := &sessionProc{
		cmd:    cmd,
		client: newClient(stdin, stderr),
		cancel: cancel,
	}
	proc.client.onMessage = e.onMessage(proc)

	async.GoSafe(func() {
		reader := newLineReader(stdout)
		for {
			msg, err := reader.next()
			if err != nil || msg == nil {
				break
			}
			proc.client.dispatch(msg)
		}
		proc.client.close()
	})

	// initialize
	initParams := e.cfg.initParams()
	var initRes struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
			LoadSession     bool `json:"loadSession"`
			McpCapabilities struct {
				HTTP bool `json:"http"`
				SSE  bool `json:"sse"`
			} `json:"mcpCapabilities"`
		} `json:"agentCapabilities"`
		AgentInfo struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()
	if err := proc.client.call(initCtx, "initialize", initParams, &initRes); err != nil {
		proc.close()
		return nil, errors.Wrap(err, "ACP initialize 失败", "stderr", stderr.String())
	}
	proc.agentName = initRes.AgentInfo.Title
	if proc.agentName == "" {
		proc.agentName = initRes.AgentInfo.Name
	}
	proc.agentVersion = initRes.AgentInfo.Version
	proc.httpMCP = initRes.AgentCapabilities.McpCapabilities.HTTP

	// session/new
	servers := e.mcpServers()
	sessionParams := map[string]any{
		"cwd":        e.cfg.Cwd,
		"mcpServers": servers,
	}
	var newRes struct {
		SessionID string `json:"sessionId"`
	}
	sessCtx, sessCancel := context.WithTimeout(ctx, 30*time.Second)
	defer sessCancel()
	if err := proc.client.call(sessCtx, "session/new", sessionParams, &newRes); err != nil {
		proc.close()
		return nil, errors.Wrap(err, "ACP session/new 失败", "stderr", stderr.String())
	}
	proc.sessionID = newRes.SessionID
	proc.touch()
	return proc, nil
}

// initParams 构造 initialize 请求参数(Engine.start 与 ProbeStdio 共用)。
func (c *Config) initParams() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": true, "writeTextFile": true},
			"terminal": true,
		},
		"clientInfo": map[string]string{
			"name":    c.clientInfo.Name,
			"version": c.clientInfo.Version,
		},
	}
}

// Ready 静态就位性:命令在不在 PATH 上(不启动进程,快)。
//
// **与 Probe 的分工**:Ready 只答「装没装」(状态接口用,不能慢);
// 真正「会不会说 ACP」由 ProbeStdio 的握手回答(测试连接用)。
func (c *Config) Ready() (bool, string) {
	fields := splitCommand(c.Command)
	if len(fields) == 0 {
		return false, "command 不能为空"
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		return false, "找不到命令 " + fields[0] + ": " + err.Error()
	}
	return true, "命令 " + c.Command + " 可用"
}

// mcpServers 组装挂给 agent 的 MCP 服务器。
func (e *Engine) mcpServers() []map[string]any {
	out := make([]map[string]any, 0, len(e.cfg.MCPServers))
	for i := range e.cfg.MCPServers {
		s := &e.cfg.MCPServers[i]
		payload := map[string]any{"name": s.Name}
		switch s.Type {
		case "http", "sse":
			payload["type"] = s.Type
			payload["url"] = s.URL
			// headers 是数组 [{name,value}] —— ACP 规范原文的形状。
			headers := make([]map[string]string, 0, len(s.Headers))
			for _, h := range s.Headers {
				headers = append(headers, map[string]string{"name": h.Name, "value": h.Value})
			}
			payload["headers"] = headers
		default:
			payload["command"] = s.Command
			payload["args"] = s.Args
			env := make([]map[string]string, 0, len(s.Env))
			for _, ev := range s.Env {
				env = append(env, map[string]string{"name": ev.Name, "value": ev.Value})
			}
			payload["env"] = env
		}
		out = append(out, payload)
	}
	return out
}

// onMessage 构造 agent → client 的报文处理器。
func (e *Engine) onMessage(proc *sessionProc) func(*rpcMessage) {
	return func(m *rpcMessage) {
		switch {
		case m.isNotification() && m.Method == "session/update":
			var params struct {
				Update *sessionUpdate `json:"update"`
			}
			if err := json.Unmarshal(m.Params, &params); err != nil || params.Update == nil {
				return
			}
			ev := proc.mapUpdate(params.Update)
			if ev != nil {
				proc.sinkEvent(ev)
				if ev.Type == engine.EventToolStart {
					e.auditCall(auditActionToolCall, ev.Tool, ev.Args)
				}
			}
		case m.isRequest() && m.Method == "session/request_permission":
			// **不能阻塞 reader goroutine**:否则后续 session/update 全部堵住。
			async.GoSafe(func() { e.handlePermission(proc, m) })
		case m.isRequest() && strings.HasPrefix(m.Method, "fs/"):
			e.handleFS(proc, m)
		case m.isRequest() && strings.HasPrefix(m.Method, "terminal/"):
			e.handleTerminal(proc, m)
		}
	}
}

// handlePermission 处理授权请求。
func (e *Engine) handlePermission(proc *sessionProc, m *rpcMessage) {
	var params struct {
		SessionID string `json:"sessionId"`
		ToolCall  struct {
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	_ = json.Unmarshal(m.Params, &params)

	opts := make([]engine.PermissionOption, 0, len(params.Options))
	for _, o := range params.Options {
		opts = append(opts, engine.PermissionOption{
			OptionID: o.OptionID, Name: o.Name, Kind: o.Kind,
		})
	}
	reqID := rpcID(m.ID)
	title := params.ToolCall.Title
	if title == "" {
		title = params.ToolCall.Name
	}

	// 按权限档自动决策;只有 ask 档才真的等用户。
	if optID, decided := e.autoDecide(params.ToolCall.Kind, opts); decided {
		_ = proc.client.respond(m.ID, map[string]any{
			"outcome": map[string]any{"outcome": "selected", "optionId": optID},
		})
		return
	}

	item := &permItem{
		id:        reqID,
		sessionID: params.SessionID,
		title:     title,
		kind:      params.ToolCall.Kind,
		options:   opts,
		respond: func(optionID string) error {
			// optionID 为空 = 拒绝(ACP 的 cancelled outcome)。
			if optionID == "" {
				return proc.client.respond(m.ID, map[string]any{
					"outcome": map[string]any{"outcome": "cancelled"},
				})
			}
			return proc.client.respond(m.ID, map[string]any{
				"outcome": map[string]any{"outcome": "selected", "optionId": optionID},
			})
		},
		ch: make(chan string, 1),
	}
	e.perms.add(item)
	proc.sinkEvent(&engine.Event{
		Type:         engine.EventPermission,
		PermissionID: reqID,
		Tool:         title,
		Options:      opts,
	})
	select {
	case optID := <-item.ch:
		e.auditCall(auditActionPermission, title, optID)
		_ = item.respond(optID)
	case <-time.After(5 * time.Minute):
		e.perms.remove(reqID)
		_ = proc.client.respond(m.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
	}
}

// autoDecide 按权限档自动决策。
// 返回 (optionID, true) 表示已决策,不必打扰用户。
func (e *Engine) autoDecide(kind string, opts []engine.PermissionOption) (string, bool) {
	pick := func(pred func(engine.PermissionOption) bool) (string, bool) {
		for _, o := range opts {
			if pred(o) {
				return o.OptionID, true
			}
		}
		return "", false
	}
	switch e.cfg.Permission {
	case PermissionApproveAll:
		return pick(func(o engine.PermissionOption) bool {
			return strings.HasPrefix(o.Kind, "allow")
		})
	case PermissionDeny:
		return pick(func(o engine.PermissionOption) bool {
			return strings.HasPrefix(o.Kind, "reject")
		})
	case PermissionApproveReads:
		// 只读类工具(kind 是 snake_case 的小写枚举)自动放行。
		switch kind {
		case "read", "search", "think":
			return pick(func(o engine.PermissionOption) bool {
				return strings.HasPrefix(o.Kind, "allow")
			})
		}
	}
	return "", false
}

// handleFS 处理 client 侧 fs/* 请求。
func (e *Engine) handleFS(proc *sessionProc, m *rpcMessage) {
	switch m.Method {
	case "fs/read_text_file":
		res, err := e.fs.readTextFile(m.Params)
		if err != nil {
			_ = proc.client.respondError(m.ID, -32603, err.Error())
			return
		}
		_ = proc.client.respond(m.ID, res)
	case "fs/write_text_file":
		if err := e.fs.writeTextFile(m.Params); err != nil {
			_ = proc.client.respondError(m.ID, -32603, err.Error())
			return
		}
		_ = proc.client.respond(m.ID, map[string]any{})
	default:
		_ = proc.client.respondError(m.ID, -32601, "method not found: "+m.Method)
	}
}

// handleTerminal 处理 client 侧 terminal/* 请求。
func (e *Engine) handleTerminal(proc *sessionProc, m *rpcMessage) {
	res, err := e.terms.handle(m.Method, m.Params)
	if err != nil {
		_ = proc.client.respondError(m.ID, -32603, err.Error())
		return
	}
	_ = proc.client.respond(m.ID, res)
}

// startJanitor 启动空闲回收。
func (e *Engine) startJanitor() {
	interval := time.Minute
	async.GoSafe(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-e.stop:
				return
			case <-ticker.C:
				e.reapIdle()
			}
		}
	})
}

// reapIdle 回收空闲超时的 agent 进程。
func (e *Engine) reapIdle() {
	now := time.Now()
	e.mu.Lock()
	var dead []*sessionProc
	for k, p := range e.procs {
		if now.Sub(p.lastUsed()) > e.cfg.IdleTTL {
			dead = append(dead, p)
			delete(e.procs, k)
		}
	}
	e.mu.Unlock()
	for _, p := range dead {
		async.GoSafe(func() { p.close() })
	}
}

// sessionProc 一个 agent 进程 + 一个 ACP 会话。
type sessionProc struct {
	cmd          *exec.Cmd
	client       *client
	sessionID    string
	agentName    string
	agentVersion string
	httpMCP      bool

	// cancel 终止 agent 进程(与 cmd 的 context 绑定)。
	cancel context.CancelFunc

	mapper *updateMapper

	mu        sync.Mutex
	sink      func(engine.Event)
	used      time.Time
	closeOnce sync.Once
	// turnMu 串行化同一 ACP 会话上的回合。
	//
	// **为什么**:同一个 git-ferry 会话并发发两轮时,① 共享的 sink 会被覆盖
	// (第一轮的事件流到第二轮的通道,输出错乱);② ACP 会话一次只处理一个
	// prompt,并发 prompt 本身就不合法。⇒ 排队,而不是各跑各的。
	turnMu sync.Mutex
	// inFlight 正在跑的回合数(>0 ⇒ 这个进程不能被回收,
	// 否则会把在飞的回合打断,用户看到的是「工具调用到一半断了」)。
	inFlight int
}

// beginTurn 标记一个回合开始。
func (p *sessionProc) beginTurn() {
	p.mu.Lock()
	p.inFlight++
	p.used = time.Now()
	p.mu.Unlock()
}

// endTurn 标记一个回合结束。
func (p *sessionProc) endTurn() {
	p.mu.Lock()
	if p.inFlight > 0 {
		p.inFlight--
	}
	p.mu.Unlock()
}

// busy 当前是否有回合在飞。
func (p *sessionProc) busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inFlight > 0
}

func (p *sessionProc) touch() {
	p.mu.Lock()
	p.used = time.Now()
	p.mu.Unlock()
}

func (p *sessionProc) lastUsed() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.used
}

func (p *sessionProc) setSink(f func(engine.Event)) {
	p.mu.Lock()
	p.sink = f
	p.mu.Unlock()
}

func (p *sessionProc) sinkEvent(ev *engine.Event) {
	p.mu.Lock()
	f := p.sink
	p.mu.Unlock()
	if f != nil {
		f(*ev)
	}
}

// mapUpdate 映射 session/update(带状态:记住工具名)。
func (p *sessionProc) mapUpdate(u *sessionUpdate) *engine.Event {
	p.mu.Lock()
	if p.mapper == nil {
		p.mapper = newUpdateMapper()
	}
	mapper := p.mapper
	p.mu.Unlock()
	return mapper.mapUpdate(u)
}

// close 关闭进程(幂等)。
//
// **nil 保护**:close 可能发生在进程还没建起来 / 已被回收的场合。
func (p *sessionProc) close() {
	p.closeOnce.Do(func() {
		if p.client != nil {
			p.client.close()
		}
		if p.cancel != nil {
			p.cancel()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
}
