package git_sync

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLocalDirRefHidesAbsolutePath 审计明细里的本机路径必须降级。
//
// 审计明细进**不可篡改的哈希链** —— 写进去就等于永久提交:
// 事后想改会让整条链报「被篡改」。
// 而绝对路径还会把执行机的目录结构与操作者用户名永久写进审计记录。
func TestLocalDirRefHidesAbsolutePath(t *testing.T) {
	ptr := func(s string) *string { return &s }

	ref := localDirRef(ptr("/Users/someone/private/repo"))
	assert.NotContains(t, ref, "/Users", "审计明细里不许出现本机绝对路径:%s", ref)
	assert.NotContains(t, ref, "someone", "审计明细里不许出现本机用户名:%s", ref)
	assert.True(t, strings.HasPrefix(ref, "repo#"), "应保留目录名以便对照:%s", ref)

	// 同一个路径 → 同一个 ref(可对照、可去重)。
	assert.Equal(t, ref, localDirRef(ptr("/Users/someone/private/repo")))

	// 不同路径 → 不同 ref(不会撞)。
	assert.NotEqual(t, ref, localDirRef(ptr("/Users/someone/private/other")))

	// 空值 → 空(不产出 "nil#xxxx" 这种噪声)。
	assert.Equal(t, "", localDirRef(nil))
	assert.Equal(t, "", localDirRef(ptr("")))
}
