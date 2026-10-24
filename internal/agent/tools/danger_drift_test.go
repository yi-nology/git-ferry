package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDangerToolsCoverEveryDangerGuardCall 防漂移门禁:
//
// **凡是走 dangerGuard 的工具,都必须在危险工具集合里**。
//
// 为什么需要它:危险工具的判定原先是**另一份手写清单**(DangerTools map),
// 而真正的执行路径是各工具实现里的 `r.dangerGuard(ctx, "<名字>", ...)`。
// 两份清单漂移时,后果不是「安全洞」(dangerGuard 自己有角色闸),
// 而是**工具卡住**与**清单说谎**:
//   - MCP 的 tools/list 不过滤 ⇒ 只读角色看得见写工具;
//   - system prompt 的危险工具清单缺它 ⇒ 助手不知道它需要确认。
//
// 现在危险标注写在**注册处**(repo.go 的 addDanger),本测试从**源码**里
// 把所有 dangerGuard 调用挖出来,断言它们都在集合里 ——
// 新增危险工具却忘了标注,这里立刻红。
//
// 静态扫描(而不是运行时枚举):dangerGuard 只在**真正调用**时才暴露自己,
// 静态扫描才能覆盖「注册了但没人调过」的工具。
func TestDangerToolsCoverEveryDangerGuardCall(t *testing.T) {
	guarded := dangerGuardCallers(t)

	danger := map[string]bool{}
	for _, name := range DangerToolNames() {
		danger[name] = true
	}

	var missing []string
	for name := range guarded {
		if !danger[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing,
		"这些工具走了 dangerGuard(需要用户确认),却没在注册处标注为危险工具:"+
			"新增危险工具请用 addDanger 标注")
}

// TestDangerToolNamesNonEmpty 钉住「集合不是空的」——
// 解析失效时上面的测试会空跑通过。
func TestDangerToolNamesNonEmpty(t *testing.T) {
	names := DangerToolNames()
	require.NotEmpty(t, names, "危险工具集合不能为空")
	for _, n := range names {
		assert.True(t, IsDangerTool(n), "%s 应被认成危险工具", n)
	}
}

// dangerGuardCallers 扫描本包源码,返回**所有走 dangerGuard 的工具名**。
func dangerGuardCallers(t *testing.T) map[string]bool {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "取不到本测试文件路径")
	dir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	out := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "dangerGuard" {
				return true
			}
			// dangerGuard(ctx, "工具名", args, fn) —— 工具名是第 2 个参数。
			if len(call.Args) < 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			out[strings.Trim(lit.Value, `"`)] = true
			return true
		})
	}
	return out
}
