package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPending_ConsumeOnce(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()

	token, err := s.SetPending("run_task", `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	args, err := s.ConsumePending("run_task", token)
	require.NoError(t, err)
	assert.Equal(t, `{"task_key":"t1"}`, args)

	// 一次性消费
	_, err = s.ConsumePending("run_task", token)
	assert.ErrorIs(t, err, ErrNoPending)
}

func TestConsumePending_Mismatch(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()
	token, err := s.SetPending("run_task", `{"task_key":"t1"}`)
	require.NoError(t, err)

	// 严格语义:任何一次消费尝试(含失败)都销毁 pending,防令牌爆破/重放
	_, err = s.ConsumePending("test_repo_connection", token)
	assert.ErrorIs(t, err, ErrConfirmMismatch)

	_, err = s.ConsumePending("run_task", token)
	assert.ErrorIs(t, err, ErrNoPending)
}

func TestConsumePending_Expired(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()
	token, err := s.SetPending("run_task", `{}`)
	require.NoError(t, err)
	// 直接把过期时间拨回
	s.pending.ExpiresAt = time.Now().Add(-time.Second)
	_, err = s.ConsumePending("run_task", token)
	assert.ErrorIs(t, err, ErrConfirmExpired)
}

func TestSetPending_Overwrite(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()
	_, err := s.SetPending("run_task", `{"task_key":"t1"}`)
	require.NoError(t, err)
	token2, err := s.SetPending("run_task", `{"task_key":"t2"}`) // 新请求覆盖旧请求
	require.NoError(t, err)
	args, err := s.ConsumePending("run_task", token2)
	require.NoError(t, err)
	assert.Contains(t, args, "t2")
}

func TestLastConfirm_TakeOnce(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()

	name, _, _ := s.LastConfirm()
	assert.Empty(t, name) // 未设置时全空

	s.SetLastConfirm("run_task", "tok1", `{"task_key":"t1"}`)
	name, token, args = s.LastConfirm()
	assert.Equal(t, "run_task", name)
	assert.Equal(t, "tok1", token)
	assert.Contains(t, args, "t1")

	// 取即清空
	name, _, _ = s.LastConfirm()
	assert.Empty(t, name)
}
