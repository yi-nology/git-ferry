package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/oekit/concurrency/async"

	"github.com/yi-nology/goagent-kernel/checkpoint"
	"github.com/yi-nology/goagent-kernel/contract"
	"github.com/yi-nology/goagent-kernel/invocation"
	"github.com/yi-nology/goagent-runtime/chat"

	"github.com/yi-nology/git-ferry/internal/agent/agentstore"
	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// captureModel 包住 eino FakeModel,顺路捕获每次模型调用收到的消息,
// 用于断言「恢复后模型看到了既有历史(含工具结果)」。
type captureModel struct {
	*toolstest.FakeModel
	mu   sync.Mutex
	seen [][]*schema.Message
}

func (c *captureModel) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	c.mu.Lock()
	c.seen = append(c.seen, in)
	c.mu.Unlock()
	return c.FakeModel.Generate(ctx, in, opts...)
}

func (c *captureModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.mu.Lock()
	c.seen = append(c.seen, in)
	c.mu.Unlock()
	sr, sw := schema.Pipe[*schema.Message](2)
	async.GoSafe(func() {
		defer sw.Close()
		msg, err := c.FakeModel.Generate(ctx, in, opts...)
		if err != nil {
			sw.Send(nil, err)
			return
		}
		sw.Send(msg, nil)
	})
	return sr, nil
}

// seenCount 模型被调用的次数。
func (c *captureModel) seenCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// lastSeen 最后一次调用的消息(role:content 串)。
func (c *captureModel) lastSeen() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.seen) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range c.seen[len(c.seen)-1] {
		sb.WriteString(string(m.Role))
		sb.WriteString(":")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// drainAIEvents 收事件直到 done/error(超时 5s 防挂)。
func drainAIEvents(t *testing.T, ch <-chan engine.Event) []engine.Event {
	t.Helper()
	var out []engine.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
			if ev.Type == engine.EventDone || ev.Type == engine.EventError {
				return out
			}
		case <-time.After(5 * time.Second):
			t.Fatal("事件流 5s 未收尾")
			return out
		}
	}
}

// newTestSession 构造挂 store 的会话(固定 ID,供跨重启复用)。
func newTestSession(t *testing.T, id string) *Session {
	t.Helper()
	return &Session{ID: id, store: NewSessionStore(30*time.Minute, 20)}
}

// TestGoagentEngineBasicRound 基础回合:直接终答 → delta + done,
// display 历史追加 assistant。
func TestGoagentEngineBasicRound(t *testing.T) {
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{Content: "仓库一切正常"})
	reg := tools.NewRegistry(toolstest.NewMock())
	eng, err := NewGoagentEngineWithModel(&Config{}, fm, reg)
	if err != nil {
		t.Fatalf("构造引擎: %v", err)
	}
	sess := newTestSession(t, "sess-basic")
	ch, cancel, err := eng.Run(context.Background(), sess, engine.Input{Message: "查一下"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer cancel()
	evs := drainAIEvents(t, ch)
	var (
		gotDelta bool
		gotDone  bool
	)
	for _, ev := range evs {
		switch ev.Type {
		case engine.EventDelta:
			gotDelta = ev.Content == "仓库一切正常"
		case engine.EventDone:
			gotDone = true
		case engine.EventError:
			t.Fatalf("意外错误事件: %s", ev.Content)
		}
	}
	if !gotDelta || !gotDone {
		t.Fatalf("事件不全: delta=%v done=%v", gotDelta, gotDone)
	}
}

// TestGoagentEngineToolRound 工具回合:模型先调 list_repos 再终答;
// 断言 tool_start/tool_end 事件与 tool_end 携带工具结果。
func TestGoagentEngineToolRound(t *testing.T) {
	mock := toolstest.NewMock()
	mock.Repos = []*corebridge.Repo{{Name: "r1"}}
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "list_repos", ToolArgs: `{}`})
	fm.Append(toolstest.FakeStep{Content: "查到 1 个仓库"})
	reg := tools.NewRegistry(mock)
	eng, err := NewGoagentEngineWithModel(&Config{}, fm, reg)
	if err != nil {
		t.Fatalf("构造引擎: %v", err)
	}
	sess := newTestSession(t, "sess-tool")
	ch, cancel, err := eng.Run(context.Background(), sess, engine.Input{Message: "列一下仓库"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer cancel()
	evs := drainAIEvents(t, ch)
	var (
		started bool
		ended   bool
	)
	for _, ev := range evs {
		switch ev.Type {
		case engine.EventToolStart:
			started = ev.Tool == "list_repos"
		case engine.EventToolEnd:
			ended = ev.Tool == "list_repos" && strings.Contains(ev.Result, "r1")
		case engine.EventError:
			t.Fatalf("意外错误事件: %s", ev.Content)
		}
	}
	if !started || !ended {
		t.Fatalf("工具事件不全: start=%v done=%v", started, ended)
	}
}

// TestGoagentEngineResumeAfterRestart 跨重启恢复(本方案的核心增益):
// 进程「重启」(引擎关闭重建,同一 SQLite 库与会话 ID)后,
//   - 模型看到既有历史(含工具结果)——检查点快照生效;
//   - 工具不再被执行——历史里有结果,loop 不重放。
func TestGoagentEngineResumeAfterRestart(t *testing.T) {
	mock := toolstest.NewMock()
	mock.Repos = []*corebridge.Repo{{Name: "r1"}}
	db := t.TempDir() + "/sessions.db"

	// ── 第一次「进程」:调工具,落检查点,然后退出 ──
	var cm1 *captureModel
	{
		fm := toolstest.NewFakeModel()
		fm.Append(toolstest.FakeStep{ToolName: "list_repos", ToolArgs: `{}`})
		fm.Append(toolstest.FakeStep{Content: "查到 1 个仓库"})
		cm1 = &captureModel{FakeModel: fm}
		eng, err := NewGoagentEngineWithModelAt(&Config{}, cm1, tools.NewRegistry(mock), db)
		if err != nil {
			t.Fatalf("构造引擎1: %v", err)
		}
		sess := newTestSession(t, "sess-resume")
		ch, cancel, err := eng.Run(context.Background(), sess, engine.Input{Message: "列仓库"})
		if err != nil {
			t.Fatalf("Run1: %v", err)
		}
		drainAIEvents(t, ch)
		cancel()
		if err := eng.Close(); err != nil {
			t.Fatalf("Close1: %v", err)
		}
		if cm1.seenCount() != 2 {
			t.Fatalf("Run1 模型应被调 2 次(要工具+出终答),实得 %d", cm1.seenCount())
		}
	}

	// ── 第二次「进程」:同一库、同一会话 ID,模型直接终答 ──
	var cm2 *captureModel
	{
		fm2 := toolstest.NewFakeModel()
		fm2.Append(toolstest.FakeStep{Content: "好的,仓库 r1 还在"})
		cm2 = &captureModel{FakeModel: fm2}
		eng2, err := NewGoagentEngineWithModelAt(&Config{}, cm2, tools.NewRegistry(mock), db)
		if err != nil {
			t.Fatalf("构造引擎2: %v", err)
		}
		defer func() { _ = eng2.Close() }()
		sess := newTestSession(t, "sess-resume")
		ch, cancel, err := eng2.Run(context.Background(), sess, engine.Input{Message: "那任务呢"})
		if err != nil {
			t.Fatalf("Run2: %v", err)
		}
		evs := drainAIEvents(t, ch)
		cancel()
		for _, ev := range evs {
			if ev.Type == engine.EventToolStart {
				t.Fatalf("重启后工具不得重放,却收到 tool_start: %s", ev.Tool)
			}
		}
		if !strings.Contains(cm2.lastSeen(), "r1") {
			t.Fatalf("重启后模型应看到既有历史(含工具结果),实际:\n%s", cm2.lastSeen())
		}
		if cm2.seenCount() != 1 {
			t.Fatalf("Run2 模型应被调 1 次,实得 %d", cm2.seenCount())
		}
	}
}

// TestGoagentEngineJournalLedger 工具回合落账本:执行前 planned、完成后
// applied 且保留结果——恢复续跑的第二道背书生效(agentloop.Config.Journal)。
func TestGoagentEngineJournalLedger(t *testing.T) {
	mock := toolstest.NewMock()
	mock.Repos = []*corebridge.Repo{{Name: "r1"}}
	db := t.TempDir() + "/ledger.db"

	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "list_repos", ToolArgs: `{}`})
	fm.Append(toolstest.FakeStep{Content: "查到了"})
	reg := tools.NewRegistry(mock)
	eng, err := NewGoagentEngineWithModelAt(&Config{}, fm, reg, db)
	if err != nil {
		t.Fatalf("构造引擎: %v", err)
	}
	defer func() { _ = eng.Close() }()

	sess := newTestSession(t, "sess-ledger")
	ch, cancel, err := eng.Run(context.Background(), sess, engine.Input{Message: "列仓库"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := drainAIEvents(t, ch)
	cancel()
	for _, ev := range evs {
		if ev.Type == engine.EventError {
			t.Fatalf("意外错误事件: %s", ev.Content)
		}
	}

	// 从同一库重开账本面,读回账目:恰一条,applied 且保留结果。
	st, err := agentstore.Open(db)
	if err != nil {
		t.Fatalf("重开库: %v", err)
	}
	defer func() { _ = st.Close() }()
	entries, err := st.Journal().List(context.Background(), "sess-ledger")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("应恰 1 条账目,实得 %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Tool != "list_repos" {
		t.Fatalf("账目工具名应为 list_repos,实得 %s", e.Tool)
	}
	if e.State != invocation.StateResultRecorded {
		t.Fatalf("账目终态应为 result_recorded(施加+回录后),实得 %s", e.State)
	}
}

// TestGoagentEngineLedgerResumeCrashWindow 账本背书的崩溃窗口恢复(账本的核心增益):
// 进程死在「账本 executing → 历史回填提交」之间 ⇒ 历史不完整(assistant 要了
// 工具、结果缺失)+ 账目停在 executing。恢复 Run:
//   - 历史校验不再 fail-closed(账本背书合法未完成调用);
//   - resumePending 按账本续跑该调用(sideeffect 闸保证不重复施加);
//   - 模型不会被再问一次那次调用。
func TestGoagentEngineLedgerResumeCrashWindow(t *testing.T) {
	ctx := context.Background()
	db := t.TempDir() + "/crash.db"

	// ── 种子:手工构造崩溃窗口状态 ──
	st, err := agentstore.Open(db)
	if err != nil {
		t.Fatalf("开种子库: %v", err)
	}
	history := []chat.Message{
		{Role: chat.RoleUser, Content: "列一下仓库"},
		{Role: chat.RoleAssistant, ToolCalls: []chat.ToolCall{{
			ID: "call-crash", Name: "list_repos", Arguments: `{}`,
		}}},
		// (工具结果缺失 = 崩溃窗口形态,上面省略即是不完整历史)
	}
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("序列化历史: %v", err)
	}
	snap := checkpoint.Snapshot{
		RunID:  "sess-crash",
		Seq:    1,
		Status: contract.StatusRunning,
		Fields: map[string]string{"history": string(raw)},
	}
	if err := st.Checkpoints().SaveIfNewer(ctx, snap); err != nil {
		t.Fatalf("种快照: %v", err)
	}
	j := st.Journal()
	if err := j.Plan(ctx, invocation.Entry{
		SessionID: "sess-crash", CallID: "call-crash",
		Tool: "list_repos", ArgsDigest: invocation.Digest([]byte(`{}`)),
		Args: []byte(`{}`), State: invocation.StatePlanned,
	}); err != nil {
		t.Fatalf("种账目: %v", err)
	}
	if _, err := j.Advance(ctx, "sess-crash", "call-crash", invocation.StateExecuting, invocation.Update{}); err != nil {
		t.Fatalf("推进 executing: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关种子库: %v", err)
	}

	// ── 恢复 Run:模型脚本只有终答(不再被问那次调用) ──
	mock := toolstest.NewMock()
	mock.Repos = []*corebridge.Repo{{Name: "r1"}}
	cm := &captureModel{FakeModel: toolstest.NewFakeModel()}
	cm.Append(toolstest.FakeStep{Content: "恢复完成,仓库 r1 还在"})
	reg := tools.NewRegistry(mock)
	eng, err := NewGoagentEngineWithModelAt(&Config{}, cm, reg, db)
	if err != nil {
		t.Fatalf("构造引擎: %v", err)
	}
	defer func() { _ = eng.Close() }()

	sess := newTestSession(t, "sess-crash")
	ch, cancel, err := eng.Run(ctx, sess, engine.Input{Message: "接着查"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := drainAIEvents(t, ch)
	cancel()

	toolStarts := 0
	for _, ev := range evs {
		if ev.Type == engine.EventToolStart && ev.Tool == "list_repos" {
			toolStarts++
		}
		if ev.Type == engine.EventError {
			t.Fatalf("恢复不应报错(账本背书),实得: %s", ev.Content)
		}
	}
	if toolStarts != 1 {
		t.Fatalf("账本续跑应恰好执行 1 次 list_repos,实得 %d", toolStarts)
	}
	if !strings.Contains(cm.lastSeen(), "r1") {
		t.Fatalf("模型应看到续跑回填的工具结果,实际:\n%s", cm.lastSeen())
	}

	// 账目收口:result_recorded。
	st2, err := agentstore.Open(db)
	if err != nil {
		t.Fatalf("重开库: %v", err)
	}
	defer func() { _ = st2.Close() }()
	entries, err := st2.Journal().List(ctx, "sess-crash")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].State != invocation.StateResultRecorded {
		t.Fatalf("账目应收口为 result_recorded,实得 %+v", entries)
	}
}
