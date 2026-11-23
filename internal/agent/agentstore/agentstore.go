// Package agentstore 会话检查点存储(SQLite WAL)与进程内协调面。
//
// 为什么不用 goagent-runtime 的 sqliteadapter:git-ferry-core/model 生产代码
// 静态引入 glebarez sqlite 驱动(DSN 支持 sqlite 形态),而 sqliteadapter 引
// modernc——两者都在 init 里 sql.Register("sqlite") 同名注册,同二进制 init
// 即炸(实测:agent 测试 panic)。本包用同一 glebarez 驱动实现 kernel
// checkpoint.StoreCAS,序号单调判据仍在 kernel Committer(SeqGate +
// SaveIfNewer 契约);副作用闸走 kernel coord 适配器(判定逻辑单源),
// 后端用 coord 内存后端(单进程声明),详见 memory.go。
package agentstore

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/oekit/pkg/errors"

	"github.com/yi-nology/goagent-kernel/checkpoint"
	"github.com/yi-nology/goagent-kernel/coord"
	"github.com/yi-nology/goagent-kernel/sideeffect"

	_ "github.com/glebarez/go-sqlite"
)

// ensureSchema 建表语句(幂等)。
const ensureSchema = `
CREATE TABLE IF NOT EXISTS agent_snapshots (
	run_id  TEXT PRIMARY KEY,
	seq     INTEGER NOT NULL,
	status  TEXT NOT NULL DEFAULT '',
	done    TEXT NOT NULL DEFAULT '[]',
	fields  TEXT NOT NULL DEFAULT '{}'
)`

// Store 会话检查点存储(SQLite WAL)。实现 kernel checkpoint.StoreCAS。
type Store struct {
	db *sql.DB
}

// Open 打开(或建)SQLite 会话库并初始化 schema。
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("agent 会话库路径为空(匿名库 = 历史无处可寻)")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.Wrap(err, "打开 agent 会话库失败", "path", path)
	}
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, errors.Wrapf(err, "设置 pragma 失败: %s", pragma)
		}
	}
	if _, err := db.Exec(ensureSchema); err != nil {
		_ = db.Close()
		return nil, errors.Wrap(err, "初始化 schema 失败")
	}
	return &Store{db: db}, nil
}

// Checkpoints 进度快照面(Store 本体实现 StoreCAS)。
func (s *Store) Checkpoints() checkpoint.StoreCAS { return s }

// Effects 副作用闸面(kernel coord 判据 + 进程内内存后端,见 memory.go)。
func (s *Store) Effects() sideeffect.Store {
	return mustStoreAdapter(coord.NewMemoryBackend())
}

// SaveIfNewer 仅当已持久化序号严格小于 s.Seq 才写入,否则 ErrStaleSeq 且不写。
// BEGIN IMMEDIATE 串行化并发写;单进程内还有 loop 的 SeqGate 前置过滤。
// Seq == 0 必须拒(非真实提交,契约条款)。
func (s *Store) SaveIfNewer(ctx context.Context, s2 checkpoint.Snapshot) error {
	if s2.Seq == 0 {
		return checkpoint.ErrStaleSeq
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "开启事务失败")
	}
	defer func() { _ = tx.Rollback() }()
	var cur uint64
	err = tx.QueryRowContext(ctx, `SELECT seq FROM agent_snapshots WHERE run_id = ?`, s2.RunID).Scan(&cur)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		cur = 0
	case err != nil:
		return errors.Wrap(err, "读取快照序号失败", "run", s2.RunID)
	}
	if s2.Seq <= cur {
		return checkpoint.ErrStaleSeq
	}
	doneJSON, err := json.Marshal(s2.Done)
	if err != nil {
		return errors.Wrap(err, "序列化 done 失败")
	}
	fieldsJSON, err := json.Marshal(s2.Fields)
	if err != nil {
		return errors.Wrap(err, "序列化 fields 失败")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO agent_snapshots (run_id, seq, status, done, fields) VALUES (?, ?, ?, ?, ?)`,
		s2.RunID, s2.Seq, s2.Status, string(doneJSON), string(fieldsJSON)); err != nil {
		return errors.Wrap(err, "写入快照失败", "run", s2.RunID)
	}
	return tx.Commit()
}

// Load 无记录返回 checkpoint.ErrNotFound(不得返回零值对)。
func (s *Store) Load(ctx context.Context, runID string) (checkpoint.Snapshot, error) {
	var (
		seq    uint64
		status string
		done   string
		fields string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT seq, status, done, fields FROM agent_snapshots WHERE run_id = ?`, runID,
	).Scan(&seq, &status, &done, &fields)
	if errors.Is(err, sql.ErrNoRows) {
		return checkpoint.Snapshot{}, checkpoint.ErrNotFound
	}
	if err != nil {
		return checkpoint.Snapshot{}, errors.Wrap(err, "读取会话快照失败")
	}
	snap := checkpoint.Snapshot{RunID: runID, Seq: seq, Status: status}
	if err := json.Unmarshal([]byte(done), &snap.Done); err != nil {
		return checkpoint.Snapshot{}, errors.Wrap(err, "反序列化 done 失败")
	}
	if err := json.Unmarshal([]byte(fields), &snap.Fields); err != nil {
		return checkpoint.Snapshot{}, errors.Wrap(err, "反序列化 fields 失败")
	}
	return snap, nil
}

// Close 关库。
func (s *Store) Close() error { return s.db.Close() }

// 编译期断言:Store 实现 kernel StoreCAS 契约。
var _ checkpoint.StoreCAS = (*Store)(nil)
