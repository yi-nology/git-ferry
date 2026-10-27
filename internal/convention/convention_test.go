package convention

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot 从本包位置往上找仓库根(有 go.mod 的那一层)。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("找不到仓库根(go.mod)")
	return ""
}

// goFiles 列出仓库里的 Go 源文件(排除测试文件与第三方目录)。
func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == "node_modules" || base == ".git" ||
				base == "testdata" || base == "data" || base == "output" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil // 测试文件不强制(测试里 go func 无所谓)
		}
		out = append(out, path)
		return nil
	})
	require.NoError(t, err)
	return out
}

// stripComments 去掉注释(行注释 + 块注释),只留代码。
//
// 规则针对的是**代码**:文档/注释里写「禁止裸 go func」不该把自己判为违规。
func stripComments(src string) string {
	var out strings.Builder
	inBlock := false
	for _, ln := range strings.Split(src, "\n") {
		if inBlock {
			if i := strings.Index(ln, "*/"); i >= 0 {
				ln = ln[i+2:]
				inBlock = false
			} else {
				continue
			}
		}
		// 去掉行注释(简单处理:不在字符串里的 // 才算注释 —— 本仓代码不
		// 在行注释里写含 // 的字符串,足够用)。
		if i := strings.Index(ln, "//"); i >= 0 {
			ln = ln[:i]
		}
		for {
			i := strings.Index(ln, "/*")
			if i < 0 {
				break
			}
			j := strings.Index(ln[i+2:], "*/")
			if j < 0 {
				ln = ln[:i]
				inBlock = true
				break
			}
			ln = ln[:i] + ln[i+2+j+2:]
		}
		out.WriteString(ln)
		out.WriteString("\n")
	}
	return out.String()
}

// linesWith 找出命中正则的行(返回 "相对路径:行号:内容")。
func linesWith(files []string, root string, re *regexp.Regexp) []string {
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for i, ln := range strings.Split(stripComments(string(b)), "\n") {
			if re.MatchString(ln) {
				rel, _ := filepath.Rel(root, f)
				out = append(out, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(ln))
			}
		}
	}
	return out
}

// TestConvention_禁止裸goroutine 裸 go func 的 panic **没人接** ⇒
// 直接带走整个进程(oekit/concurrency/async 的 GoSafe 收编 panic)。
//
// 对 MCP/AI 这类**长驻进程**尤其致命:一个后台 goroutine panic ⇒
// 这个会话的两个服务一起没了。
func TestConvention_禁止裸goroutine(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root)
	hits := linesWith(files, root, regexp.MustCompile(`(?m)\bgo\s+func\s*\(`))
	assert.Empty(t, hits,
		"发现裸 go func:一律改用 concurrency/async(GoSafe / GoSafeLabeled / GoSafeWithErr)")
}

// TestConvention_错误处理必须用oekit的errors stdlib errors 与 oekit 的
// errors **同名** ⇒ 两套错误体系混用时,`Is`/`As` 判定会跨体系失效
// (oekit 的红线:类型同一性,一个 import 即可)。
//
// 判据:源码里不得 import stdlib "errors";要用 oekit 的
// github.com/oekit/pkg/errors(New/Is/As/Wrap/BizError 同型)。
func TestConvention_错误处理必须用oekit的errors(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root)
	hits := linesWith(files, root, regexp.MustCompile(`(?m)^\s*"errors"\s*$`))
	assert.Empty(t, hits,
		"发现 stdlib \"errors\" 导入:改用 github.com/oekit/pkg/errors(同名同型,Is/As 一个 import 即可)")
}

// TestConvention_禁止fmtErrorf包w %w 包装统一走 errors.Wrap(err, msg):
// 显式 nil-safe(不会出 \"%!w(<nil>)\"),且能挂 kv 上下文。
func TestConvention_禁止fmtErrorf包w(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root)
	hits := linesWith(files, root, regexp.MustCompile(`fmt\.Errorf\([^)]*%w`))
	assert.Empty(t, hits,
		"发现 fmt.Errorf(...%w...):改用 errors.Wrap(err, msg)(oekit/pkg/errors)")
}

// TestConvention_日志必须走logx门面 直连 slog 打日志 ⇒ 绕过了统一门面:
// 级别、脱敏、trace_id、审计留痕全都失效,且与包级 logger 出口不一致。
//
// 允许的唯一例外:logx 接线本身(slog.Default().Handler() 复用同一个 handler)。
func TestConvention_日志必须走logx门面(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root)
	hits := linesWith(files, root, regexp.MustCompile(`\bslog\.(Info|Error|Warn|Debug)\(`))
	assert.Empty(t, hits,
		"发现直连 slog 打日志:改用包级 logger(= logx.Delegating(\"组件名\"))")
}

// TestConvention_包级logger必须用Delegating 包级 var 在 init 时求值 ⇒
// 直接捕获 logx.Default() 会永远绑在 init 那一刻的后端上,
// 消费方随后的 SetDefault 对它无效(日志进黑洞)。
// Delegating 是**调用时**解析,自动跟随。
func TestConvention_包级logger必须用Delegating(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root)
	hits := linesWith(files, root, regexp.MustCompile(`=\s*logx\.Default\(\)`))
	assert.Empty(t, hits,
		"发现包级 logger 捕获 logx.Default():改用 logx.Delegating(\"组件名\")(调用时解析,跟随后端切换)")
}
