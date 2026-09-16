package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_CreateGet(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	s := st.Create()
	assert.NotEmpty(t, s.ID)

	got, err := st.Get(s.ID)
	require.NoError(t, err)
	assert.Same(t, s, got)
}

func TestSessionStore_GetNotFound(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 20)
	_, err := st.Get("nope")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionStore_TTLExpiry(t *testing.T) {
	st := NewSessionStore(10*time.Millisecond, 20)
	s := st.Create()
	time.Sleep(30 * time.Millisecond)
	_, err := st.Get(s.ID)
	assert.ErrorIs(t, err, ErrSessionNotFound) // 过期即删
}

func TestSessionStore_AppendRoundCap(t *testing.T) {
	st := NewSessionStore(30*time.Minute, 3) // 3 轮 = 6 条
	s := st.Create()
	for i := 0; i < 5; i++ {
		st.Append(s, "user", "q")
		st.Append(s, "assistant", "a")
	}
	require.Len(t, s.Messages, 6)
	// 最旧的被淘汰,保留最近 3 轮
	assert.Equal(t, "q", s.Messages[0].Content)
	assert.Equal(t, "a", s.Messages[5].Content)
}
