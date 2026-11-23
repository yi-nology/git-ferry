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

	"github.com/yi-nology/goagent-kernel/checkpoint"
	"github.com/yi-nology/goagent-kernel/coord"
	"github.com/yi-nology/goagent-kernel/sideeffect"
)

// Faces agent 的两个存储面(检查点 + 副作用闸)。
type Faces struct {
	Checks  checkpoint.StoreCAS
	Effects sideeffect.Store
}

// NewMemory 进程内存储面(coord 内存后端 + 内存检查点)。
// 供测试与「无持久化目录」的退化装配;进程退出即丢历史。
func NewMemory() *Faces {
	return &Faces{
		Checks:  &memChecks{snaps: map[string]checkpoint.Snapshot{}},
		Effects: mustStoreAdapter(coord.NewMemoryBackend()),
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
