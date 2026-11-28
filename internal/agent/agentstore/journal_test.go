package agentstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yi-nology/goagent-kernel/invocation"
	"github.com/yi-nology/goagent-kernel/invocation/invocationtest"

	"github.com/yi-nology/git-ferry/internal/agent/agentstore"
)

func invocationEntry(sessionID, callID, tool, args string) invocation.Entry {
	return invocation.Entry{
		SessionID:  sessionID,
		CallID:     callID,
		Tool:       tool,
		ArgsDigest: invocation.Digest([]byte(args)),
		Args:       []byte(args),
		State:      invocation.StatePlanned,
	}
}

func invocationStatePlanned() invocation.State   { return invocation.StatePlanned }
func invocationStateExecuting() invocation.State { return invocation.StateExecuting }
func invocationStateApplied() invocation.State   { return invocation.StateApplied }
func invocationUpdateEmpty() invocation.Update   { return invocation.Update{} }
func invocationUpdateResult(r string) invocation.Update {
	return invocation.Update{Result: []byte(r)}
}

// TestJournalSQLiteContract kernel invocationtest 共享契约套件(15 判据)钉住
// SQLite 账本实现:身份幂等/冲突、转移合法性(Predecessors 单源)、终态冻结、
// 会话隔离、并发推进单赢家——判据不另写第二套。
func TestJournalSQLiteContract(t *testing.T) {
	invocationtest.Run(t, func(t *testing.T) invocation.Store {
		st, err := agentstore.Open(filepath.Join(t.TempDir(), "journal.db"))
		if err != nil {
			t.Fatalf("打开账本库: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st.Journal()
	})
}

// TestJournalMemoryContract 同一套判据钉内存账本(测试面)。
func TestJournalMemoryContract(t *testing.T) {
	invocationtest.Run(t, func(t *testing.T) invocation.Store {
		return agentstore.NewMemory().Journal
	})
}

// TestJournalPlannedBeforeSideEffect 账本先于副作用落库的顺序判据:
// 落账即可读回 planned 态,推进到 applied 后账目保留结果(恢复续跑依据)。
func TestJournalPlannedBeforeSideEffect(t *testing.T) {
	ctx := context.Background()
	st, err := agentstore.Open(filepath.Join(t.TempDir(), "journal.db"))
	if err != nil {
		t.Fatalf("打开账本库: %v", err)
	}
	defer func() { _ = st.Close() }()
	j := st.Journal()

	// Plan ⇒ Get:planned 态即持久化(副作用前的账)。
	e := invocationEntry("sess-j", "call-j", "list_repos", `{}`)
	if err := j.Plan(ctx, e); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	got, err := j.Get(ctx, "sess-j", "call-j")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != invocationStatePlanned() {
		t.Fatalf("落账后应为 planned,实得 %s", got.State)
	}

	// planned → executing → applied,结果保留。
	if _, err := j.Advance(ctx, "sess-j", "call-j", invocationStateExecuting(), invocationUpdateEmpty()); err != nil {
		t.Fatalf("Advance executing: %v", err)
	}
	out, err := j.Advance(ctx, "sess-j", "call-j", invocationStateApplied(), invocationUpdateResult(`{"total":1}`))
	if err != nil {
		t.Fatalf("Advance applied: %v", err)
	}
	if string(out.Result) != `{"total":1}` {
		t.Fatalf("applied 应保留结果,实得 %s", out.Result)
	}
}
