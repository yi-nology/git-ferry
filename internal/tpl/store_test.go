package tpl

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_UpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "templates.json"))
	require.NoError(t, err)

	tpl, err := st.Upsert(&Template{
		Name: "nightly", Spec: Spec{Cron: "0 2 * * *"},
		Tags: []string{"backup"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, tpl.ID)

	got, err := st.Get(tpl.ID)
	require.NoError(t, err)
	assert.Equal(t, "nightly", got.Name)
	assert.Equal(t, "0 2 * * *", got.Spec.Cron)

	// 重开文件应恢复
	st2, err := Open(filepath.Join(dir, "templates.json"))
	require.NoError(t, err)
	got2, err := st2.Get(tpl.ID)
	require.NoError(t, err)
	assert.Equal(t, "nightly", got2.Name)

	require.NoError(t, st.Delete(tpl.ID))
	_, err = st.Get(tpl.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}


func TestStore_ListByTag(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.json"))
	require.NoError(t, err)
	_, err = st.Upsert(&Template{Name: "a", Tags: []string{"nightly", "backup"}})
	require.NoError(t, err)
	_, err = st.Upsert(&Template{Name: "b", Tags: []string{"hotfix"}})
	require.NoError(t, err)

	got := st.ListByTag("nightly")
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Name)
	assert.Empty(t, st.ListByTag("missing"))
}
