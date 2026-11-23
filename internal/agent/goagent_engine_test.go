package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/oekit/concurrency/async"

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
