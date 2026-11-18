package acp

import (
	"encoding/json"
	"fmt"
	"github.com/oekit/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/oekit/concurrency/async"
)

// terminalHandler 实现 **client 侧**的 terminal/* 能力。
//
// ACP 里 terminal 是可选能力(agent 通过 initialize 的 clientCapabilities.terminal
// 声明需要它)。实测 mcode 不用它,但 claude-agent-acp / codex-acp 一类会要求。
//
// 沙箱:命令的工作目录固定在 root 内;输出有上限,防把磁盘写爆。
//
// **工作目录必须过沙箱判定**(与 fs/* 同一份边界):此前 cwd 是**直接用
// agent 给的**——文档写着「固定在 root 内」,实现却没管 ⇒ agent 报一个
// 根外的 cwd 就能把命令挪到沙箱外跑。
type terminalHandler struct {
	*sandbox

	mu    sync.Mutex
	terms map[string]*termProc
	seq   atomic.Int64
	// max 同时存活的终端上限(测试可调小)。
	max int
	// permission 外部 agent 授权档:deny 全拒,approve_reads 只允许
	// 保守只读命令;ask/approve_all 维持原行为。
	permission string
}

// maxTerminals 同时存活的终端上限。
//
// 终端只在 agent 调 terminal/release 时才释放 —— agent 不释放的话
// 这里的 map 就会一直涨(内存泄漏)。给个上限,超了直接拒:
// 让 agent 自己决定先放掉哪个,比悄悄回收它正在用的终端安全。
const maxTerminals = 16

func newTerminalHandler(root string) (*terminalHandler, error) {
	sb, err := newSandbox(root)
	if err != nil {
		// 根目录不可用 ⇒ **不能退化成「无沙箱」**(那等于把边界打开),
		// 直接报错,由调用方如实告诉用户「这个 agent 起不来」。
		return nil, err
	}
	return newTerminalHandlerWith(sb), nil
}

// newTerminalHandlerWith 用已建好的沙箱构造处理器(与 fs/* 共用同一份边界)。
func newTerminalHandlerWith(sb *sandbox) *terminalHandler {
	return &terminalHandler{
		sandbox: sb, terms: make(map[string]*termProc),
		max: maxTerminals, permission: PermissionAsk,
	}
}

// authorizeCommand 按授权档约束 terminal/create。
//
// 此前 terminal/* 完全不看 cfg.Permission:deny / approve_reads 档的 agent
// 仍能通过 terminal/create 执行任意命令,授权档形同虚设。
func (h *terminalHandler) authorizeCommand(command string, args []string) error {
	switch h.permission {
	case PermissionDeny:
		return errors.New("当前授权档为 deny,terminal 能力不可用")
	case PermissionApproveReads:
		if !isReadOnlyTerminalCommand(command, args) {
			return errors.New("当前授权档为 approve_reads,仅允许只读命令")
		}
		if !argsStayInRoot(args) {
			return errors.New("只读命令不得引用工作目录之外的路径")
		}
	}
	return nil
}

// isReadOnlyTerminalCommand 保守只读命令白名单。
//
// 只放行确定无写入副作用的命令;git branch/tag/remote 仅允许列式参数。
func isReadOnlyTerminalCommand(command string, args []string) bool {
	switch filepath.Base(command) {
	case "ls", "cat", "head", "tail", "wc", "grep", "rg", "pwd", "stat", "file":
		return true
	case "git":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "status", "diff", "log", "show", "rev-parse", "ls-files", "blame", "grep":
			return true
		case "branch", "tag", "remote":
			if len(args) == 1 {
				return true
			}
			for _, a := range args[1:] {
				switch a {
				case "-a", "-r", "-v", "--list", "--verbose":
				default:
					return false
				}
			}
			return true
		}
		return false
	case "go":
		return len(args) == 1 && (args[0] == "version" || args[0] == "env")
	}
	return false
}

// argsStayInRoot 拒绝绝对路径与向上穿越:只读命令也不能读沙箱外的文件。
func argsStayInRoot(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "/") || a == ".." ||
			strings.HasPrefix(a, "../") || strings.Contains(a, "/../") {
			return false
		}
	}
	return true
}

// termProc 一个终端进程。
type termProc struct {
	cmd      *exec.Cmd
	out      *ringBuffer
	done     chan struct{}
	exitCode atomic.Int32
	killed   atomic.Bool
}

// handle 分发 terminal/* 方法。
func (h *terminalHandler) handle(method string, raw json.RawMessage) (any, error) {
	switch method {
	case "terminal/create":
		return h.create(raw)
	case "terminal/output":
		return h.output(raw)
	case "terminal/wait_for_exit":
		return h.waitForExit(raw)
	case "terminal/kill":
		return h.kill(raw)
	case "terminal/release":
		return h.release(raw)
	default:
		return nil, errors.New(fmt.Sprintf("method not found: %s", method))
	}
}

// create 创建终端。
func (h *terminalHandler) create(raw json.RawMessage) (any, error) {
	var p struct {
		Command         string              `json:"command"`
		Args            []string            `json:"args"`
		Env             []map[string]string `json:"env"`
		Cwd             string              `json:"cwd"`
		OutputByteLimit int                 `json:"outputByteLimit"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errors.Wrap(err, "参数解析失败")
	}
	if p.Command == "" {
		return nil, errors.New("command 不能为空")
	}
	if err := h.authorizeCommand(p.Command, p.Args); err != nil {
		return nil, err
	}
	cwd := h.root
	if p.Cwd != "" {
		// **工作目录必须过沙箱判定**:agent 给的 cwd 直接用 ⇒
		// 报个根外路径就能把命令挪到沙箱外。
		resolved, err := h.resolveCwd(p.Cwd)
		if err != nil {
			return nil, err
		}
		cwd = resolved
	}
	limit := p.OutputByteLimit
	if limit <= 0 {
		limit = 256 * 1024
	}
	maxAlive := h.max
	if maxAlive <= 0 {
		maxAlive = maxTerminals
	}
	h.mu.Lock()
	if len(h.terms) >= maxAlive {
		h.mu.Unlock()
		return nil, errors.New(fmt.Sprintf("同时存活的终端已达上限(%d),请先 terminal/release 释放", maxAlive))
	}
	h.mu.Unlock()

	cmd := exec.Command(p.Command, p.Args...) //nolint:gosec // 命令由 agent 指定,工作目录受限于 root
	cmd.Dir = cwd
	// **必须显式设置 cmd.Env**:为 nil 时子进程会继承父进程全部环境,
	// 包括 ENCRYPTION_KEY / GIT_SYNC_* / GITFERRY_*。与外部 agent 主进程
	// 一致地先 scrub,再叠加 agent 显式声明的环境变量。
	env := scrubEnv(os.Environ())
	for _, kv := range p.Env {
		env = append(env, kv["name"]+"="+kv["value"])
	}
	cmd.Env = env
	buf := newRingBuffer(limit)
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "启动终端失败")
	}
	tp := &termProc{cmd: cmd, out: buf, done: make(chan struct{})}
	async.GoSafe(func() {
		err := cmd.Wait()
		if err != nil {
			var ee *exec.ExitError
			if ok := asExitError(err, &ee); ok {
				tp.exitCode.Store(int32(ee.ExitCode()))
			} else {
				tp.exitCode.Store(-1)
			}
		}
		close(tp.done)
	})
	id := fmt.Sprintf("term-%d", h.seq.Add(1))
	h.mu.Lock()
	h.terms[id] = tp
	h.mu.Unlock()
	return map[string]any{"terminalId": id}, nil
}

// output 取输出与退出状态。
func (h *terminalHandler) output(raw json.RawMessage) (any, error) {
	id, err := h.terminalID(raw)
	if err != nil {
		return nil, err
	}
	tp := h.get(id)
	if tp == nil {
		return nil, errors.New(fmt.Sprintf("终端 %s 不存在", id))
	}
	return map[string]any{
		"output":    tp.out.String(),
		"truncated": false,
		"exitStatus": map[string]any{
			"exitCode": int(tp.exitCode.Load()),
			"signal":   nil,
		},
	}, nil
}

// waitForExit 等终端退出。
func (h *terminalHandler) waitForExit(raw json.RawMessage) (any, error) {
	id, err := h.terminalID(raw)
	if err != nil {
		return nil, err
	}
	tp := h.get(id)
	if tp == nil {
		return nil, errors.New(fmt.Sprintf("终端 %s 不存在", id))
	}
	<-tp.done
	return map[string]any{
		"exitStatus": map[string]any{
			"exitCode": int(tp.exitCode.Load()),
			"signal":   nil,
		},
	}, nil
}

// kill 杀掉终端(不释放)。
func (h *terminalHandler) kill(raw json.RawMessage) (any, error) {
	id, err := h.terminalID(raw)
	if err != nil {
		return nil, err
	}
	tp := h.get(id)
	if tp == nil {
		return nil, errors.New(fmt.Sprintf("终端 %s 不存在", id))
	}
	tp.killed.Store(true)
	if tp.cmd.Process != nil {
		_ = tp.cmd.Process.Kill()
	}
	return map[string]any{}, nil
}

// release 释放终端。
func (h *terminalHandler) release(raw json.RawMessage) (any, error) {
	id, err := h.terminalID(raw)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	tp := h.terms[id]
	delete(h.terms, id)
	h.mu.Unlock()
	if tp != nil && tp.cmd.Process != nil {
		_ = tp.cmd.Process.Kill()
	}
	return map[string]any{}, nil
}

// get 取终端。
func (h *terminalHandler) get(id string) *termProc {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.terms[id]
}

// terminalID 从参数里取 terminalId。
func (h *terminalHandler) terminalID(raw json.RawMessage) (string, error) {
	var p struct {
		TerminalID string `json:"terminalId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", errors.Wrap(err, "参数解析失败")
	}
	if p.TerminalID == "" {
		return "", errors.New("terminalId 不能为空")
	}
	return p.TerminalID, nil
}

// asExitError 判断是否为 exec.ExitError。
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // exec 直接返回该具体类型
		*target = ee
		return true
	}
	return false
}

// splitCommand 把命令串拆成字段(支持单/双引号包裹)。
//
// 例:"mcode acp" → ["mcode","acp"];"'./bin/dev acp' --x" → ["./bin/dev acp","--x"]
func splitCommand(cmd string) []string {
	var (
		out   []string
		cur   strings.Builder
		quote rune
		has   bool
	)
	for _, r := range cmd {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			has = true
		case r == ' ' || r == '\t' || r == '\n':
			if cur.Len() > 0 || has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 || has {
		out = append(out, cur.String())
	}
	return out
}
