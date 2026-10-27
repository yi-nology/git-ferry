package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandbox_越界路径要拒 沙箱是外部 agent 唯一的**代码层**边界
// (工作目录即边界,更强的隔离靠部署侧容器)⇒ 判定必须严格。
func TestSandbox_越界路径要拒(t *testing.T) {
	root := t.TempDir()
	sb, err := newSandbox(root)
	require.NoError(t, err)

	// 根内的相对路径:放行
	// (用 sb.root:macOS 上 /var → /private/var,根是**解析后**的路径)
	require.NoError(t, sb.inside(filepath.Join(sb.root, "a", "b.txt")))

	// 根外:拒绝
	err = sb.inside(filepath.Join(filepath.Dir(sb.root), "outside.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "越界")
}

// TestSandbox_软链逃逸要拒 工作区里一个指向根外的软链 ⇒ 解析后越界。
//
// **这是软链版的沙箱逃逸**:路径文本看着在根内,实际读的是根外的文件。
func TestSandbox_软链逃逸要拒(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "id_rsa")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600))

	link := filepath.Join(root, "innocent.txt")
	require.NoError(t, os.Symlink(secret, link))

	sb, err := newSandbox(root)
	require.NoError(t, err)

	// 经 resolve 解析(会跟随软链)⇒ 必须被拒
	_, err = sb.resolve(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "越界")
}

// TestSandbox_严格档拒绝显式dotdot 宽松档的 "a/../b" 抵消是**纯文本语义,
// 看不见符号链接** ⇒ 沙箱用严格档:相对路径在**拼根之前**就判,
// 显式出现的 ".." 段一律拒。
func TestSandbox_严格档拒绝显式dotdot(t *testing.T) {
	root := t.TempDir()
	// 造一个 "a" 目录:若它是软链,"a/../b" 的真实落点可能落在根外。
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a"), 0o750))

	sb, err := newSandbox(root)
	require.NoError(t, err)

	// 相对路径 "a/../b":严格档直接拒(宽松档会抵消成 "b" 并放行)
	_, err = sb.resolve("a/../b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "越界")

	// 普通相对路径:放行
	_, err = sb.resolve("a/ok.txt")
	require.NoError(t, err)
}

// TestTerminalHandler_cwd必须在沙箱内 **回归**:此前 terminal/create 的 cwd
// 是**直接用 agent 给的**(文档写「固定在 root 内」,实现没管)
// ⇒ agent 报个根外的 cwd 就能把命令挪到沙箱外跑。
func TestTerminalHandler_cwd必须在沙箱内(t *testing.T) {
	root := t.TempDir()
	h, err := newTerminalHandler(root)
	require.NoError(t, err)

	// 根外的 cwd:必须拒
	_, err = h.create(json.RawMessage(`{"command":"pwd","cwd":"/"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "越界")

	// 根内的 cwd:放行(命令跑在沙箱里)
	res, err := h.create(json.RawMessage(`{"command":"pwd","cwd":"` + root + `"}`))
	require.NoError(t, err)
	require.NotNil(t, res)
}
