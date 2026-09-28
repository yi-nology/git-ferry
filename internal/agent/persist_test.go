package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistResult_SmallPassThrough(t *testing.T) {
	SetPersistDir(t.TempDir())
	out := persistResult("list", "short")
	assert.Equal(t, "short", out)
}

func TestPersistResult_LargePersists(t *testing.T) {
	dir := t.TempDir()
	SetPersistDir(dir)
	big := strings.Repeat("x", resultBudget+100)
	out := persistResult("list", big)
	assert.Contains(t, out, "<persisted-output>")
	assert.Contains(t, out, "saved to:")
	// 文件存在
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	_, err = os.Stat(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)
}
