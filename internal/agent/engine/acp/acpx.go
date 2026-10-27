package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oekit/pkg/errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/oekit/concurrency/async"
	"github.com/oekit/logx"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// logger 是本包的日志出口(经 logx 门面)。
//
// **用 Delegating 而不是 Default()**:包级 var 在 init 时求值,
// 直接捕获 Default() 会永远绑在 init 那一刻的默认 logger 上,
// 消费方随后的 SetDefault 对它无效;Delegating 是**调用时**解析,
// 自动跟随 main() 里装好的后端。
var logger = logx.Delegating("acp")

// AcpxEngine 通过 acpx CLI 驱动外部 coding agent。
//
// acpx 是 ACP 的无头 CLI 客户端:它自己当 ACP client,把整段 JSON-RPC 流
// 打到 stdout。本引擎只做两件事:起进程、读流、把 session/update 映射成事件。
//
// **能力边界(如实告知,不要假装有)**:
//   - 拿不到 session/request_permission(权限决策在 acpx 侧按档位自动处理);
//   - 挂不了 git-ferry 自己的 MCP(要看各 adapter 自己的 MCP 配置)。
//
// 需要这两项 ⇒ 用 stdio transport(Engine)。
type AcpxEngine struct {
	cfg Config

	// audit 外部 agent 行为的审计出口(可选)。
	audit AuditFunc

	mu sync.Mutex
	// created 记录本进程内已建过 acpx 会话的 git-ferry 会话。
	// acpx 的会话存在磁盘上(~/.acpx/sessions),git-ferry 重启后仍在,
	// 所以「建过」只是省一次调用,不是正确性依赖 —— 失败时会自愈。
	created map[string]bool
	// usedAt 每个 acpx 会话最后一次使用的时间(空闲回收用)。
	usedAt map[string]time.Time
	// stop 停掉空闲回收 goroutine。
	stop     chan struct{}
	stopOnce sync.Once
}

// NewAcpxEngine 构造 acpx 后端。
func NewAcpxEngine(cfg *Config) (*AcpxEngine, error) {
	cfg.Transport = TransportACPX
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	e := &AcpxEngine{
		cfg:     *cfg,
		created: make(map[string]bool),
		usedAt:  make(map[string]time.Time),
		stop:    make(chan struct{}),
	}
	e.startJanitor()
	return e, nil
}

// ID 后端标识。
func (e *AcpxEngine) ID() string { return TransportACPX }

// Label 展示名。
func (e *AcpxEngine) Label() string {
	if e.cfg.Agent != "" {
		return "外部 coding agent (" + e.cfg.Agent + " / acpx)"
	}
	return "外部 coding agent (acpx)"
}

// ModelName 展示用 agent 名。
func (e *AcpxEngine) ModelName() string {
	if e.cfg.Agent != "" {
		return e.cfg.Agent
	}
	return e.cfg.Command
}

// Close 关闭本进程建过的 acpx 会话。
//
// **为什么必须关**:acpx 的会话是**磁盘上的持久记录**(~/.acpx/sessions/),
// 不关就一直攒 —— git-ferry 每个会话建一个,长期跑就把磁盘喂饱了。
func (e *AcpxEngine) Close() error {
	e.stopOnce.Do(func() { close(e.stop) })
	e.mu.Lock()
	names := make([]string, 0, len(e.created))
	for name := range e.created {
		names = append(names, name)
	}
	e.created = make(map[string]bool)
	e.usedAt = make(map[string]time.Time)
	e.mu.Unlock()
	for _, name := range names {
		e.closeSession(name)
	}
	return nil
}

// startJanitor 启动空闲回收。
//
// **为什么需要**:acpx 的会话是磁盘上的持久记录。git-ferry 的会话 30 分钟
// 就过期了,但 acpx 的会话不会自己消失 —— 不回收的话,长期运行的进程
// 会在 ~/.acpx/sessions/ 下攒出一堆孤儿记录。
func (e *AcpxEngine) startJanitor() {
	async.GoSafe(func() {
		ticker := time.NewTicker(time.Minute)
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

// reapIdle 回收空闲超时的 acpx 会话。
func (e *AcpxEngine) reapIdle() {
	ttl := e.cfg.IdleTTL
	if ttl <= 0 {
		ttl = DefaultIdleTTL
	}
	now := time.Now()
	e.mu.Lock()
	var stale []string
	for name, used := range e.usedAt {
		if now.Sub(used) > ttl {
			stale = append(stale, name)
		}
	}
	e.mu.Unlock()
	for _, name := range stale {
		e.closeSession(name)
		e.mu.Lock()
		delete(e.created, name)
		delete(e.usedAt, name)
		e.mu.Unlock()
	}
}

// closeSession 关掉一个 acpx 会话(尽力而为:失败只告警)。
func (e *AcpxEngine) closeSession(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := e.baseArgs()
	args = append(args, e.agentToken()...)
	args = append(args, "sessions", "close", name)
	cmd := exec.CommandContext(ctx, e.cfg.Binary, args...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = e.cfg.Cwd
	cmd.Env = scrubbedEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("关闭 acpx 会话失败", "session", name, "error", err, "output", strings.TrimSpace(string(out)))
	}
}

// Ready 静态就位性:acpx 在不在 + agent 在不在(快,不启动进程)。
func (e *AcpxEngine) Ready() (bool, string) {
	if _, err := exec.LookPath(e.cfg.Binary); err != nil {
		return false, "找不到 acpx(" + e.cfg.Binary + "): " + err.Error() +
			" —— 请先 npm install -g acpx"
	}
	name := e.cfg.Agent
	if name == "" {
		// 自定义命令:查它本身
		if e.cfg.Command != "" {
			fields := splitCommand(e.cfg.Command)
			if len(fields) > 0 {
				if _, err := exec.LookPath(fields[0]); err != nil {
					return false, "找不到命令 " + fields[0] + ": " + err.Error()
				}
			}
		}
		return true, "acpx 就绪"
	}
	// 内置 agent 名:已知以本地可执行文件安装的才查 PATH
	// (其余走 npx adapter,不在 PATH 上也正常)。
	if bin, ok := localAgentBins[name]; ok {
		if _, err := exec.LookPath(bin); err != nil {
			return false, "找不到 " + name + "(" + bin + "): " + err.Error()
		}
	}
	return true, "acpx 与 " + name + " 就绪"
}

// localAgentBins 已知以本地可执行文件形式安装的 agent。
// 其余内置名(codex/claude/kilocode 等)走 npx adapter,不在 PATH 上也正常。
var localAgentBins = map[string]string{
	"mcode": "mcode", "claude": "claude", "codex": "codex", "gemini": "gemini",
	"opencode": "opencode", "qwen": "qwen", "cursor-agent": "cursor-agent",
	"copilot": "copilot", "kimi": "kimi", "iflow": "iflow", "openclaw": "openclaw",
	"pi": "pi", "droid": "droid", "trae": "trae", "zeroclaw": "zeroclaw",
}

// SetAudit 注入审计出口(可选)。
func (e *AcpxEngine) SetAudit(f AuditFunc) { e.audit = f }

// auditCall 记一条审计(失败只告警,不影响主流程)。
func (e *AcpxEngine) auditCall(action, tool, detail string) {
	if e.audit == nil {
		return
	}
	e.audit(context.Background(), action, tool, detail)
}

// Run 执行一轮对话。
func (e *AcpxEngine) Run(ctx context.Context, sess engine.Session, in engine.Input) (<-chan engine.Event, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan engine.Event, 64)

	emit := func(ev engine.Event) {
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	}

	sessionName := sess.SessionID()
	async.GoSafe(func() {
		defer close(out)
		defer cancel()
		text, err := e.runTurn(ctx, sessionName, in.Message, emit)
		if err != nil {
			emit(engine.Event{Type: engine.EventError, Content: err.Error()})
			return
		}
		sess.Append("assistant", text)
		emit(engine.Event{Type: engine.EventDone})
	})
	return out, cancel, nil
}

// runTurn 跑一轮:起 acpx → 读流 → 收尾。
func (e *AcpxEngine) runTurn(ctx context.Context, sessionName, prompt string, emit func(engine.Event)) (string, error) {
	// 第一次用这个会话时先建 acpx 会话(agent 侧才有真正的多轮记忆)。
	if !e.hasSession(sessionName) {
		if err := e.ensureSession(ctx, sessionName); err != nil {
			return "", err
		}
	}
	text, err := e.execPrompt(ctx, sessionName, prompt, emit)
	if err == nil {
		return text, nil
	}
	// 会话可能已被 acpx 清掉(磁盘上的会话被手工删了/过期了)⇒ 建一次再重试。
	if !isNoSessionError(err) {
		return "", err
	}
	e.forgetSession(sessionName)
	if err2 := e.ensureSession(ctx, sessionName); err2 != nil {
		return "", errors.Wrap(err, "重建 acpx 会话也失败", "rebuild_err", err2)
	}
	return e.execPrompt(ctx, sessionName, prompt, emit)
}

// hasSession 本进程内是否建过。
func (e *AcpxEngine) hasSession(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.created[name]
}

// forgetSession 忘掉已建标记。
func (e *AcpxEngine) forgetSession(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.created, name)
}

// ensureSession 建 acpx 会话。
func (e *AcpxEngine) ensureSession(ctx context.Context, name string) error {
	// MCP 挂载也要在建会话时给:acpx 从配置里读 mcpServers,
	// 会话记录里若没有它,后续 prompt 时那条链就接不上。
	mcpArgs, cleanup, err := e.mcpArgs()
	if err != nil {
		return err
	}
	defer cleanup()

	args := e.baseArgs()
	args = append(args, mcpArgs...)
	args = append(args, e.agentToken()...)
	args = append(args, "sessions", "new", "--name", name)
	// 二进制与参数由管理员在配置里指定(不是外部请求输入)。
	cmd := exec.CommandContext(ctx, e.cfg.Binary, args...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = e.cfg.Cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("acpx 建会话失败(%s): %v: %s", acpxBinary, err, strings.TrimSpace(string(out)))
	}
	e.mu.Lock()
	e.created[name] = true
	e.usedAt[name] = time.Now()
	e.mu.Unlock()
	return nil
}

// mcpArgs 把 MCP 清单写成 acpx 能吃的临时配置文件,返回 `--mcp-config <path>` 参数。
//
// 没有配 MCP ⇒ 返回空参数(acpx 用它自己的项目/全局配置)。
// 文件跑完即删(见 writeMCPConfig 的说明:里面有只读 key 的专用凭据)。
func (e *AcpxEngine) mcpArgs() ([]string, func(), error) {
	path, cleanup, err := writeMCPConfig(e.cfg.MCPServers)
	if err != nil {
		return nil, func() {}, err
	}
	if path == "" {
		return nil, cleanup, nil
	}
	return []string{"--mcp-config", path}, cleanup, nil
}

// execPrompt 跑一轮提示,读 stdout 的 ACP 流。
func (e *AcpxEngine) execPrompt(ctx context.Context, sessionName, prompt string, emit func(engine.Event)) (string, error) {
	e.mu.Lock()
	e.usedAt[sessionName] = time.Now()
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	// MCP 挂载:acpx 只能从文件读 MCP ⇒ 写一份临时配置(跑完即删)。
	mcpArgs, cleanup, err := e.mcpArgs()
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := e.baseArgs()
	args = append(args, mcpArgs...)
	args = append(args, e.agentToken()...)
	args = append(args, e.cfg.Args...)
	args = append(args, "-s", sessionName, prompt)

	// 二进制与参数由管理员在配置里指定(不是外部请求输入)。
	cmd := exec.CommandContext(ctx, e.cfg.Binary, args...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = e.cfg.Cwd
	// 环境净化:acpx 自己不碰密钥,但它会再去起 agent 进程 —— 密钥不能从它这条链漏出去。
	cmd.Env = scrubbedEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "acpx stdout 管道失败")
	}
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("acpx 启动失败(%s)", e.cfg.Binary))
	}

	mapper := newUpdateMapper()
	var text strings.Builder
	var streamErr error

	reader := newLineReader(stdout)
	for {
		msg, err := reader.next()
		if err != nil {
			streamErr = errors.Wrap(err, "读取 acpx 输出失败")
			break
		}
		if msg == nil {
			break // 流结束
		}
		if msg.Error != nil {
			// acpx 用 JSON-RPC error 回执(比如 NO_SESSION)。
			streamErr = msg.Error
			break
		}
		if msg.isNotification() && msg.Method == "session/update" {
			var params struct {
				Update *sessionUpdate `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil || params.Update == nil {
				continue
			}
			if ev := mapper.mapUpdate(params.Update); ev != nil {
				if ev.Type == engine.EventDelta {
					text.WriteString(ev.Content)
				}
				if ev.Type == engine.EventToolStart {
					e.auditCall(auditActionToolCall, ev.Tool, ev.Args)
				}
				emit(*ev)
			}
			continue
		}
		if msg.isResponse() {
			// 回合结果:{stopReason:"end_turn"}。
			var res struct {
				StopReason string `json:"stopReason"`
			}
			if err := json.Unmarshal(msg.Result, &res); err == nil && res.StopReason != "" {
				break
			}
		}
	}

	waitErr := cmd.Wait()
	if streamErr != nil {
		return "", streamErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("acpx 执行失败: %v: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return text.String(), nil
}

// baseArgs acpx 的公共参数。
func (e *AcpxEngine) baseArgs() []string {
	args := []string{"--format", "json"}
	if e.cfg.Cwd != "" {
		args = append(args, "--cwd", e.cfg.Cwd)
	}
	switch e.cfg.Permission {
	case PermissionApproveAll:
		args = append(args, "--approve-all")
	case PermissionApproveReads:
		args = append(args, "--approve-reads")
	case PermissionDeny:
		args = append(args, "--deny-all")
	case PermissionAsk:
		// acpx **非交互**(它的 stdin/stdout 都被我们占着)⇒ 弹不出授权卡。
		// 它的 nonInteractivePermissions 默认是 deny ⇒ 「问」在这里等于「拒绝」。
		// 配置校验阶段已经拒绝这个组合(见 Config.Validate),这里只是兜底:
		// 显式给 --approve-reads(而不是"不传 flag 吃默认值"),
		// 免得 acpx 改了默认档就把行为带偏。
		args = append(args, "--approve-reads")
	}
	return args
}

// agentToken agent 选择:自定义命令走 --agent,否则用内置名。
func (e *AcpxEngine) agentToken() []string {
	if e.cfg.Command != "" {
		return []string{"--agent", e.cfg.Command}
	}
	return []string{e.cfg.Agent}
}

// isNoSessionError 判断是否「acpx 会话不存在」。
//
// 实测 acpx 的回执形如:
//
//	{"error":{"code":-32002,"message":"⚠ No acpx session found ...","data":{"acpxCode":"NO_SESSION"}}}
func isNoSessionError(err error) bool {
	if err == nil {
		return false
	}
	var re *rpcError
	if errors.As(err, &re) {
		var data struct {
			AcpxCode string `json:"acpxCode"`
		}
		if len(re.Data) > 0 && json.Unmarshal(re.Data, &data) == nil {
			return data.AcpxCode == "NO_SESSION"
		}
		return strings.Contains(re.Message, "No acpx session found")
	}
	return strings.Contains(err.Error(), "No acpx session found")
}

// RunOnce 用 acpx 跑一次**一次性**任务,返回 agent 的最终文本。
//
// 供「委派」场景使用(内置助手把一段编码任务派给外部 agent):
// 无会话、无记忆、跑完即走。
func RunOnce(ctx context.Context, cfg *Config, prompt string) (string, error) {
	cfg.Transport = TransportACPX
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// MCP 挂载:acpx 只能从文件读 MCP ⇒ 写一份临时配置(跑完即删)。
	mcpPath, mcpCleanup, err := writeMCPConfig(cfg.MCPServers)
	if err != nil {
		return "", err
	}
	defer mcpCleanup()

	args := []string{"--format", "json"}
	if cfg.Cwd != "" {
		args = append(args, "--cwd", cfg.Cwd)
	}
	if mcpPath != "" {
		args = append(args, "--mcp-config", mcpPath)
	}
	switch cfg.Permission {
	case PermissionApproveAll:
		args = append(args, "--approve-all")
	case PermissionApproveReads:
		args = append(args, "--approve-reads")
	case PermissionDeny:
		args = append(args, "--deny-all")
	case PermissionAsk:
		// 同上:非交互通道没法逐次确认,兜底走 approve-reads。
		args = append(args, "--approve-reads")
	}
	if cfg.Command != "" {
		args = append(args, "--agent", cfg.Command)
	} else {
		args = append(args, cfg.Agent)
	}
	args = append(args, cfg.Args...)
	args = append(args, "exec", prompt)

	cmd := exec.CommandContext(ctx, cfg.Binary, args...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = cfg.Cwd
	cmd.Env = scrubbedEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "acpx stdout 管道失败")
	}
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("acpx 启动失败(%s)", cfg.Binary))
	}

	mapper := newUpdateMapper()
	var text strings.Builder
	var streamErr error
	reader := newLineReader(stdout)
	for {
		msg, err := reader.next()
		if err != nil {
			streamErr = errors.Wrap(err, "读取 acpx 输出失败")
			break
		}
		if msg == nil {
			break
		}
		if msg.Error != nil {
			streamErr = msg.Error
			break
		}
		if msg.isNotification() && msg.Method == "session/update" {
			var params struct {
				Update *sessionUpdate `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil || params.Update == nil {
				continue
			}
			if ev := mapper.mapUpdate(params.Update); ev != nil && ev.Type == engine.EventDelta {
				text.WriteString(ev.Content)
			}
		}
	}
	waitErr := cmd.Wait()
	if streamErr != nil {
		return "", streamErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("acpx 执行失败: %v: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", errors.New("acpx 没有返回任何文本")
	}
	return text.String(), nil
}

// Probe 检查 acpx 是否可用(命令存在 + 能跑起来)。
//
// 用于「测试连接」:不真正发提示,只验证二进制在不在、agent 认不认。
func Probe(ctx context.Context, cfg *Config) (string, error) {
	cfg.Transport = TransportACPX
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	bin, err := exec.LookPath(cfg.Binary)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("找不到 acpx(%s):  —— 请先 npm install -g acpx", cfg.Binary))
	}
	cmd := exec.CommandContext(ctx, bin, "--version") //nolint:gosec // G204:路径来自配置
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("acpx --version 失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
