package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releaseCommitScript 发布提交助手(仓库 scripts/ 下)。
const releaseCommitScript = "../../scripts/release-commit.sh"

// newScratchRepo 起一个临时 git 仓库,返回其目录。
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) //nolint:gosec // G204:测试内固定参数调用 git
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	run("init", "-q", ".")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	return dir
}

// gitOut 在指定目录跑 git 并返回输出。
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // G204:测试内固定参数调用 git
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// runScript 在指定目录跑 release-commit.sh,返回输出与退出码。
func runScript(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs(releaseCommitScript)
	require.NoError(t, err)
	cmd := exec.Command("bash", append([]string{script}, args...)...) //nolint:gosec // G204:测试内跑临时脚本
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			require.NoError(t, err)
		}
	}
	return string(out), code
}

// TestReleaseCommit_不带上别人的staged文件 发布提交**不能**带上别的会话
// 已经 `git add` 的文件。
//
// 回归背景(P0,真实发生过):两个 agent 会话共享同一个工作树 ⇒ 共享**同一个
// .git/index**。`git commit`(不带 --only)提交的是**整个暂存区**
// ⇒ 另一会话 add 的 `docs/design/*.md` 进了我的发版提交,被我的 tag 一起发布。
//
// 这条用例是**反向验证**:把脚本里的 `--only` 去掉,本用例会变红。
func TestReleaseCommit_不带上别人的staged文件(t *testing.T) {
	dir := newScratchRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base"), 0o600))
	gitOut(t, dir, "add", "base.txt")
	gitOut(t, dir, "commit", "-qm", "init")

	// 另一会话:改了 a.txt 并 git add(进的是**共享的 index**)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a2"), 0o600))
	gitOut(t, dir, "add", "a.txt")

	// 我:新增自己的文件
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600))
	gitOut(t, dir, "add", "b.txt")

	msgFile := filepath.Join(dir, "msg.txt")
	require.NoError(t, os.WriteFile(msgFile, []byte("chore: 只提交 b.txt\n"), 0o600))

	out, code := runScript(t, dir, msgFile, "b.txt")
	require.Equal(t, 0, code, "脚本输出:%s", out)

	files := gitOut(t, dir, "show", "--name-only", "--format=", "HEAD")
	assert.Equal(t, "b.txt", files,
		"提交里不能出现别的会话 staged 的文件(实际=%q)\n脚本输出:%s", files, out)

	// 别人的文件仍留在 index 里,没被吞掉
	staged := gitOut(t, dir, "diff", "--cached", "--name-only")
	assert.Contains(t, staged, "a.txt", "别人的 staged 文件必须原样留在 index 里")
}

// TestReleaseCommit_自检能抓出越界文件 --check 模式必须能独立发现
// "HEAD 里混进了不在允许路径内的文件"。
//
// 这是 `--only` 之外的第二道防线:万一有人后来把脚本改成不带 --only
// (或用 "." 这种宽路径),自检要能立刻发现。
func TestReleaseCommit_自检能抓出越界文件(t *testing.T) {
	dir := newScratchRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600))
	gitOut(t, dir, "add", "a.txt", "b.txt")
	gitOut(t, dir, "commit", "-qm", "越界提交")

	// 允许路径只含 b.txt ⇒ 必须报越界并非 0 退出
	out, code := runScript(t, dir, "--check", "b.txt")
	assert.NotEqual(t, 0, code, "越界时必须非 0 退出(输出:%s)", out)
	assert.Contains(t, out, "越界文件", "必须明说哪个文件越界(输出:%s)", out)

	// 允许路径含全部 ⇒ 通过
	out, code = runScript(t, dir, "--check", "a.txt", "b.txt")
	assert.Equal(t, 0, code, "路径齐全时必须通过(输出:%s)", out)
}

// TestReleaseCommit_提交后自检通过 正常路径下脚本必须成功退出并给出提示。
func TestReleaseCommit_提交后自检通过(t *testing.T) {
	dir := newScratchRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base"), 0o600))
	gitOut(t, dir, "add", "base.txt")
	gitOut(t, dir, "commit", "-qm", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600))
	gitOut(t, dir, "add", "b.txt")

	msgFile := filepath.Join(dir, "msg.txt")
	require.NoError(t, os.WriteFile(msgFile, []byte("chore: 提交 b\n"), 0o600))

	out, code := runScript(t, dir, msgFile, "b.txt")
	require.Equal(t, 0, code, "脚本输出:%s", out)
	assert.Contains(t, out, "只含指定路径")
	assert.Equal(t, "b.txt", gitOut(t, dir, "show", "--name-only", "--format=", "HEAD"))
}
