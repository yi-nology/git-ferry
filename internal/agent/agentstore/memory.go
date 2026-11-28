// memory.go 进程内协调面:副作用闸(coord 判据单源)+ 内存检查点(测试用)。
//
// Effects 的判据全部在 kernel coord.StoreAdapter(判定顺序/三判合一/
// 执行意图都在那里);后端用 coord 内存后端并显式声明仅单进程。检查点历史
// 已持久化工具结果,恢复时不重执行已完成调用;崩溃窗口(施加成功、检查点
// 未落)由 kernel 的 fail-closed 历史校验兜住,见包注释。
package agentstore

import (
	"context"
	"sync"

	"github.com/oekit/pkg/errors"

	"github.com/yi-nology/goagent-kernel/checkpoint"
	"github.com/yi-nology/goagent-kernel/coord"
	"github.com/yi-nology/goagent-kernel/invocation"
	"github.com/yi-nology/goagent-kernel/sideeffect"
)

// Faces agent 的三个存储面(检查点 + 副作用闸 + 调用账本)。
type Faces struct {
	Checks  checkpoint.StoreCAS
	Effects sideeffect.Store
	Journal invocation.Store
}

// NewMemory 进程内存储面(coord 内存后端 + 内存检查点 + 内存账本)。
// 供测试与「无持久化目录」的退化装配;进程退出即丢历史。
func NewMemory() *Faces {
	return &Faces{
		Checks:  &memChecks{snaps: map[string]checkpoint.Snapshot{}},
		Effects: mustStoreAdapter(coord.NewMemoryBackend()),
		Journal: newMemJournal(),
	}
}

// mustStoreAdapter 装配 kernel 判据适配器;内存后端声明了单进程,
// 装配失败属编程错误。
func mustStoreAdapter(b coord.Backend) sideeffect.Store {
	sa, err := coord.NewStoreAdapter(b)
	if err != nil {
		panic("agentstore: coord StoreAdapter 装配失败: " + err.Error())
	}
	return sa
}

// memChecks 内存检查点(测试用):SaveIfNewer/Load 语义与 SQL 版同源。
type memChecks struct {
	mu    sync.Mutex
	snaps map[string]checkpoint.Snapshot
}

// SaveIfNewer 仅当已持久化序号严格小于 s.Seq 才写入。
func (m *memChecks) SaveIfNewer(_ context.Context, s checkpoint.Snapshot) error {
	if s.Seq == 0 {
		return checkpoint.ErrStaleSeq
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.snaps[s.RunID]
	if ok && s.Seq <= cur.Seq {
		return checkpoint.ErrStaleSeq
	}
	m.snaps[s.RunID] = s.Clone()
	return nil
}

// Load 无记录返回 ErrNotFound。
func (m *memChecks) Load(_ context.Context, runID string) (checkpoint.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.snaps[runID]
	if !ok {
		return checkpoint.Snapshot{}, checkpoint.ErrNotFound
	}
	return s.Clone(), nil
}

var _ checkpoint.StoreCAS = (*memChecks)(nil)

// memJournal 内存账本(测试用):语义与 SQL 版同源——Plan 幂等/身份冲突、
// Advance 经 kernel CanTransition/Predecessors 单源、0 行回读判幂等或非法。
type memJournal struct {
	mu    sync.Mutex
	seq   uint64
	books map[string]map[string]*invocation.Entry // session → callID → entry
}

func newMemJournal() invocation.Store {
	return &memJournal{books: map[string]map[string]*invocation.Entry{}}
}

// Plan 幂等落账:同 (session,call) 同身份 ⇒ 无操作;异身份 ⇒ ErrIdentityConflict。
func (m *memJournal) Plan(_ context.Context, e invocation.Entry) error { //nolint:gocritic // invocation.Store 契约要求值语义
	if e.SessionID == "" || e.CallID == "" {
		return errors.New("agentstore: 账目身份不完整(会话与调用 ID 不得为空)")
	}
	if !invocation.Known(e.State) {
		return errors.New("agentstore: 账目状态不在内核词表内")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	book := m.books[e.SessionID]
	if book == nil {
		book = map[string]*invocation.Entry{}
		m.books[e.SessionID] = book
	}
	if cur, ok := book[e.CallID]; ok {
		if cur.Tool != e.Tool || cur.ArgsDigest != e.ArgsDigest {
			return errors.Wrap(invocation.ErrIdentityConflict, "同一调用的身份与首次登记不一致(那是另一个动作)")
		}
		return nil
	}
	saved := e
	m.seq++
	saved.Seq = m.seq
	book[e.CallID] = &saved
	return nil
}

// Get 取一条账目;不存在 ⇒ ErrNotFound。
func (m *memJournal) Get(_ context.Context, sessionID, callID string) (invocation.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.books[sessionID][callID]
	if !ok {
		return invocation.Entry{}, errors.Wrap(invocation.ErrNotFound, "账目不存在")
	}
	return *e, nil
}

// List 会话全部账目(seq 升序)。
func (m *memJournal) List(_ context.Context, sessionID string) ([]invocation.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	book := m.books[sessionID]
	out := make([]invocation.Entry, 0, len(book))
	for _, e := range book {
		out = append(out, *e)
	}
	for i := 1; i < len(out); i++ {
		for k := i; k > 0 && out[k].Seq < out[k-1].Seq; k-- {
			out[k], out[k-1] = out[k-1], out[k]
		}
	}
	return out, nil
}

// Advance 经 kernel 判据单源推进;同状态幂等,非法转移拒。
func (m *memJournal) Advance(_ context.Context, sessionID, callID string, to invocation.State, u invocation.Update) (invocation.Entry, error) {
	if !invocation.Known(to) {
		return invocation.Entry{}, errors.Wrap(invocation.ErrIllegalTransition, "目标状态不在内核词表内")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.books[sessionID][callID]
	if !ok {
		return invocation.Entry{}, errors.Wrap(invocation.ErrNotFound, "账目不存在")
	}
	if e.State == to {
		return *e, nil
	}
	for _, p := range invocation.Predecessors(to) {
		if e.State == p {
			e.State = to
			if u.Result != nil {
				e.Result = u.Result
			}
			if u.Reason != "" {
				e.Reason = u.Reason
			}
			return *e, nil
		}
	}
	return invocation.Entry{}, errors.Wrap(invocation.ErrIllegalTransition,
		"当前状态不允许该转移", "from", string(e.State), "to", string(to))
}
