package memory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_RememberRecallForget(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "m.json"))
	require.NoError(t, err)

	e, err := st.Remember(KindPattern, "auth 失败先查 token 是否过期", []string{"auth", "token"})
	require.NoError(t, err)
	require.NotEmpty(t, e.ID)

	// 同内容更新而非新增
	e2, err := st.Remember(KindPattern, "auth 失败先查 token 是否过期", []string{"retry"})
	require.NoError(t, err)
	assert.Equal(t, e.ID, e2.ID)
	assert.Equal(t, 2, e2.UseCount)

	got := st.Recall("token", 10)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Content, "token")

	assert.True(t, st.Forget(e.ID))
	assert.Empty(t, st.Recall("token", 10))
}

func TestStore_ManifestAndPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.json")
	st, err := Open(path)
	require.NoError(t, err)
	_, err = st.Remember(KindPreference, "优先看中文面板", nil)
	require.NoError(t, err)

	st2, err := Open(path)
	require.NoError(t, err)
	assert.Contains(t, st2.Manifest(5), "优先看中文面板")
}
