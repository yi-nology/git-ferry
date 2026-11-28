package agentstore

import (
	"context"
	"database/sql"
	"strings"

	"github.com/oekit/pkg/errors"

	"github.com/yi-nology/goagent-kernel/invocation"
)

// agent_invocations 工具调用账本表(seq=AUTOINCREMENT,UNIQUE(session,call)),
// 与 kernel/runtime 参考实现同构;账本面刻意**不参与跨面批次**——账本必须
// 在副作用之前落库(账本续跑的前提),批次提交点在批次之后。
const ensureJournalSchema = `
CREATE TABLE IF NOT EXISTS agent_invocations (
	seq         INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id  TEXT    NOT NULL,
	call_id     TEXT    NOT NULL,
	tool        TEXT    NOT NULL DEFAULT '',
	args_digest TEXT    NOT NULL DEFAULT '',
	args        BLOB,
	state       TEXT    NOT NULL,
	result      BLOB,
	reason      TEXT    NOT NULL DEFAULT '',
	UNIQUE (session_id, call_id)
)`

// journalCols 账目列(读写共用一份,两处不漂)。
const journalCols = `session_id, call_id, tool, args_digest, args, state, result, reason, seq`

// journalStore 账本存储(glebarez SQLite;kernel invocation.Store)。
// 判据单源:身份冲突/转移合法性全部调 kernel invocation.CanTransition/
// Predecessors/Known,这里只是 DML 胶水(与 runtime sqliteadapter/journal.go
// 同构,方言适配 glebarez)。
type journalStore struct {
	db *sql.DB
}

// Journal 工具调用账本面。走自己的事务,不参与任何跨面批次。
func (s *Store) Journal() invocation.Store {
	return &journalStore{db: s.db}
}

// Plan 落一条待执行调用。INSERT ON CONFLICT DO NOTHING:冲突回读比对身份
// (tool + args_digest),一致幂等,不一致 ErrIdentityConflict。
func (j *journalStore) Plan(ctx context.Context, e invocation.Entry) error { //nolint:gocritic // invocation.Store 契约要求值语义
	if e.SessionID == "" || e.CallID == "" {
		return errors.New("agentstore: 账目身份不完整(会话与调用 ID 不得为空)")
	}
	if !invocation.Known(e.State) {
		return errors.New("agentstore: 账目状态不在内核词表内")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "开启事务失败")
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO agent_invocations (session_id, call_id, tool, args_digest, args, state, result, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (session_id, call_id) DO NOTHING`,
		e.SessionID, e.CallID, e.Tool, e.ArgsDigest, e.Args, string(e.State), e.Result, e.Reason)
	if err != nil {
		return errors.Wrap(err, "账目落库失败", "call", e.CallID)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "账目落库行数读取失败")
	}
	if n > 0 {
		return tx.Commit()
	}
	var tool, digest string
	if err := tx.QueryRowContext(ctx,
		`SELECT tool, args_digest FROM agent_invocations WHERE session_id = ? AND call_id = ?`,
		e.SessionID, e.CallID).Scan(&tool, &digest); err != nil {
		return errors.Wrap(err, "账目冲突后回读失败")
	}
	if tool != e.Tool || digest != e.ArgsDigest {
		return errors.Wrap(invocation.ErrIdentityConflict,
			"同一调用的身份与首次登记不一致(那是另一个动作)")
	}
	return tx.Commit()
}

// Get 取一条账目;不存在 ⇒ ErrNotFound。
func (j *journalStore) Get(ctx context.Context, sessionID, callID string) (invocation.Entry, error) {
	var (
		e   invocation.Entry
		st  string
		seq int64
	)
	err := j.db.QueryRowContext(ctx,
		`SELECT `+journalCols+` FROM agent_invocations WHERE session_id = ? AND call_id = ?`,
		sessionID, callID).Scan(&e.SessionID, &e.CallID, &e.Tool, &e.ArgsDigest, &e.Args, &st, &e.Result, &e.Reason, &seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return invocation.Entry{}, errors.Wrap(invocation.ErrNotFound, "账目不存在")
		}
		return invocation.Entry{}, errors.Wrap(err, "账目读取失败")
	}
	e.State = invocation.State(st)
	e.Seq = uint64(seq)
	return e, nil
}

// List 列出会话的全部账目(seq 升序)。
func (j *journalStore) List(ctx context.Context, sessionID string) ([]invocation.Entry, error) {
	rows, err := j.db.QueryContext(ctx,
		`SELECT `+journalCols+` FROM agent_invocations WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, errors.Wrap(err, "账目列举失败")
	}
	defer func() { _ = rows.Close() }()
	out := make([]invocation.Entry, 0, 8)
	for rows.Next() {
		var (
			e   invocation.Entry
			st  string
			seq int64
		)
		if err := rows.Scan(&e.SessionID, &e.CallID, &e.Tool, &e.ArgsDigest, &e.Args, &st, &e.Result, &e.Reason, &seq); err != nil {
			return nil, errors.Wrap(err, "账目行扫描失败")
		}
		e.State = invocation.State(st)
		e.Seq = uint64(seq)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "账目行读取失败")
	}
	return out, nil
}

// Advance 推进账目到目标状态:一条带条件 UPDATE(WHERE state IN 前驱集,
// kernel Predecessors 单源),比较与写入同语句;0 行 ⇒ 回读判幂等或非法转移。
// BEGIN IMMEDIATE 语义下两执行者竞争同一条账目恰一个赢家。
func (j *journalStore) Advance(ctx context.Context, sessionID, callID string, to invocation.State, u invocation.Update) (invocation.Entry, error) {
	if !invocation.Known(to) {
		return invocation.Entry{}, errors.Wrap(invocation.ErrIllegalTransition,
			"目标状态不在内核词表内(读不懂的目标不许放行)")
	}
	preds := invocation.Predecessors(to)
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return invocation.Entry{}, errors.Wrap(err, "开启事务失败")
	}
	defer func() { _ = tx.Rollback() }()
	var out invocation.Entry
	if len(preds) > 0 {
		ph := make([]string, 0, len(preds))
		args := []any{string(to), u.Result, u.Reason, sessionID, callID}
		for _, p := range preds {
			ph = append(ph, "?")
			args = append(args, string(p))
		}
		// 占位符串由 len(preds) 决定(全部是 "?"),无外部输入进 SQL 文本。
		res, err := tx.ExecContext(ctx,
			"UPDATE agent_invocations SET state = ?, result = ?, reason = ? "+ //nolint:gosec // 占位符只由前驱集长度生成
				"WHERE session_id = ? AND call_id = ? AND state IN ("+strings.Join(ph, ",")+")",
			args...)
		if err != nil {
			return invocation.Entry{}, errors.Wrap(err, "账目推进失败")
		}
		n, err := res.RowsAffected()
		if err != nil {
			return invocation.Entry{}, errors.Wrap(err, "账目推进行数读取失败")
		}
		if n > 0 {
			out, err = journalGetInTx(ctx, tx, sessionID, callID)
			if err != nil {
				return invocation.Entry{}, err
			}
			if err := tx.Commit(); err != nil {
				return invocation.Entry{}, errors.Wrap(err, "账目推进提交失败")
			}
			return out, nil
		}
	}
	cur, err := journalGetInTx(ctx, tx, sessionID, callID)
	if err != nil {
		return invocation.Entry{}, err
	}
	if cur.State == to {
		if err := tx.Commit(); err != nil {
			return invocation.Entry{}, errors.Wrap(err, "账目推进提交失败")
		}
		return cur, nil
	}
	return invocation.Entry{}, errors.Wrap(invocation.ErrIllegalTransition,
		"当前状态不允许该转移", "from", string(cur.State), "to", string(to))
}

// journalGetInTx 事务内回读一条账目。
func journalGetInTx(ctx context.Context, tx *sql.Tx, sessionID, callID string) (invocation.Entry, error) {
	var (
		e   invocation.Entry
		st  string
		seq int64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT `+journalCols+` FROM agent_invocations WHERE session_id = ? AND call_id = ?`,
		sessionID, callID).Scan(&e.SessionID, &e.CallID, &e.Tool, &e.ArgsDigest, &e.Args, &st, &e.Result, &e.Reason, &seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return invocation.Entry{}, errors.Wrap(invocation.ErrNotFound, "账目不存在")
		}
		return invocation.Entry{}, errors.Wrap(err, "账目读取失败")
	}
	e.State = invocation.State(st)
	e.Seq = uint64(seq)
	return e, nil
}

// 编译期断言:journalStore 实现 kernel invocation.Store。
var _ invocation.Store = (*journalStore)(nil)
