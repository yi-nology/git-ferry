package swagger

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// histsync 的**防漂移门禁**。
//
// 断言:OpenAPI 里的 paths 集合**覆盖**实际注册的 histsync 路由集合(方法 + 路径)。
//
// 以后有人加/改 histsync 路由(biz/router/git_sync/git_sync.go)却忘了
// 更新契约(cmd/apidoc/histsync.go),这里就会红。
//
// 为什么不用 hertz 的运行时路由表(h.Register + engine.Routes()):
//
//	internal/pkg/swagger 是底层包,为了拿一张路由表把整个 biz/handler
//	(core service / gorm / internal/agent)拖进它的单测依赖里不划算;
//	而且那个依赖链上别人正在改的包一坏,本门禁就跟 histsync 无关地红。
//
//	改为**直接解析路由源文件** biz/router/git_sync/git_sync.go ——
//	那里就是路由注册的唯一事实源,读它既不引入编译期依赖,
//	也不会因为路由是手写的(而非 hz 生成)而漏读。
const (
	// histsyncPrefix 是 histsync 路由的公共前缀。
	histsyncPrefix = "/api/v1/histsync"
)

// repoRoot 返回仓库根目录(以本测试文件所在位置反推,避免依赖测试的工作目录)。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("取不到本测试文件路径")
	}
	// thisFile = <root>/internal/pkg/swagger/histsync_contract_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("推断出的仓库根目录 %s 不对(找不到 go.mod): %v", root, err)
	}
	return root
}

// pathParamRe 匹配 hertz 的路径参数写法(:id)。
var pathParamRe = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)

// routerMethods 是 hertz 路由组上可能出现的注册方法。
var routerMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true, "CONNECT": true, "TRACE": true,
}

// registeredHistsyncRoutes 返回**实际注册**的 histsync 路由集合。
//
// 键是 "<method> <path>"(小写方法名 + OpenAPI 风格路径),值是 handler 名(报错时用)。
func registeredHistsyncRoutes(t *testing.T) map[string]string {
	t.Helper()

	routes, err := parseHistsyncRoutes(routerSource(t))
	if err != nil {
		t.Fatalf("解析路由源文件 %s 失败: %v", routerSource(t), err)
	}
	return routes
}

// routerSource 是路由注册源文件(绝对路径)。
func routerSource(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "biz", "router", "git_sync", "git_sync.go")
}

// parseHistsyncRoutes 从路由源文件里解析出 histsync 分组下注册的所有路由。
//
// 做法:先解析出所有 `x := y.Group("/seg")` 形式的分组变量并还原其完整前缀,
// 再收集 `x.METHOD("/path", ...)` 调用 —— 两者拼起来就是 hertz 实际的路由表。
func parseHistsyncRoutes(path string) (map[string]string, error) {
	return parseRoutesByPrefix(path, histsyncPrefix)
}

// parseRoutesByPrefix 解析路由源文件里**公共前缀匹配**的那些路由。
//
// 泛化自 histsync 的版本:镜像中心(mirror)是同一种「手写路由」,
// 同样需要一份防漂移门禁 —— 两边共用同一个解析器,不再各写一份。
func parseRoutesByPrefix(path, prefix string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	// 分组变量 → (父分组变量, 路径片段)
	type groupDef struct {
		parent string
		seg    string
	}
	groups := map[string]groupDef{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		lhs, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		seg, parent, ok := groupCall(assign.Rhs[0])
		if !ok {
			return true
		}
		groups[lhs.Name] = groupDef{parent: parent, seg: seg}
		return true
	})

	// 还原每个分组的完整前缀。
	full := map[string]string{}
	var resolve func(name string) string
	resolve = func(name string) string {
		if v, ok := full[name]; ok {
			return v
		}
		def, ok := groups[name]
		if !ok {
			// 根(如 r.Group("/")):父级不是分组变量,片段自身即完整路径。
			full[name] = ""
			return ""
		}
		base := ""
		if def.parent != "" {
			base = resolve(def.parent)
		}
		full[name] = joinPath(base, def.seg)
		return full[name]
	}

	routes := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		method := sel.Sel.Name
		if !routerMethods[method] {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, ok := groups[recv.Name]; !ok {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		seg, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		// 分组自身的完整前缀 + 该次注册的路径片段。
		fullPath := joinPath(resolve(recv.Name), seg)
		if !strings.HasPrefix(fullPath, prefix) {
			return true
		}
		// hertz 用 :id,OpenAPI 用 {id}。
		openAPIPath := pathParamRe.ReplaceAllString(fullPath, "{$1}")
		routes[strings.ToLower(method)+" "+openAPIPath] = handlerName(call)
		return true
	})
	return routes, nil
}

// groupCall 判断一个表达式是不是 `x.Group("/seg")` 调用。
func groupCall(expr ast.Expr) (seg, parent string, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return "", "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", "", false
	}
	seg, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", "", false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return seg, recv.Name, true
}

// joinPath 拼接路径,避免出现重复或缺失的斜杠。
func joinPath(base, seg string) string {
	if base == "" {
		if !strings.HasPrefix(seg, "/") {
			return "/" + seg
		}
		return seg
	}
	if seg == "" || seg == "/" {
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(seg, "/")
}

// handlerName 尽力取出该注册调用的 handler 名(报错信息里用)。
func handlerName(call *ast.CallExpr) string {
	var last string
	for _, arg := range call.Args {
		if sel, ok := arg.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok {
				last = pkg.Name + "." + sel.Sel.Name
				continue
			}
			last = sel.Sel.Name
		}
	}
	if last == "" {
		return "?"
	}
	return last
}

// openAPIPaths 解析一份 OpenAPI 文档,返回 path → 该 path 上的方法集合。
func openAPIPaths(t *testing.T, raw []byte) map[string]map[string]bool {
	t.Helper()

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析 OpenAPI 失败: %v", err)
	}
	out := make(map[string]map[string]bool, len(doc.Paths))
	for path, item := range doc.Paths {
		methods := make(map[string]bool, len(item))
		for method := range item {
			methods[strings.ToLower(method)] = true
		}
		out[path] = methods
	}
	return out
}

// TestHistsyncOpenAPICoversRegisteredRoutes 是 histsync 的防漂移门禁。
//
// 两份产物都要查:
//   - internal/pkg/swagger/openapi.json —— 内嵌副本,`gitferry schema` 读的就是它;
//   - docs/openapi.json —— 发布用的那份(由 `make apidoc` 从前者拷贝)。
func TestHistsyncOpenAPICoversRegisteredRoutes(t *testing.T) {
	routes := registeredHistsyncRoutes(t)
	if len(routes) == 0 {
		t.Fatalf("没从 %s 读到任何 histsync 路由 —— 路由注册方式变了?本门禁会失去意义", routerSource(t))
	}

	specs := []struct {
		name string
		raw  []byte
	}{
		{"internal/pkg/swagger/openapi.json", Spec()},
		{"docs/openapi.json", readRepoFile(t, filepath.Join(repoRoot(t), "docs", "openapi.json"))},
	}

	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			paths := openAPIPaths(t, spec.raw)

			var missing []string
			for key, handler := range routes {
				method, path, _ := strings.Cut(key, " ")
				methods, ok := paths[path]
				if !ok {
					missing = append(missing, key+" (路径未出现在 OpenAPI,handler="+handler+")")
					continue
				}
				if !methods[method] {
					missing = append(missing, key+" (方法未出现在 OpenAPI,handler="+handler+")")
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				t.Fatalf("OpenAPI 未覆盖以下已注册的 histsync 路由:\n  %s\n"+
					"契约补齐后跑 `make apidoc` 重新生成。",
					strings.Join(missing, "\n  "))
			}
		})
	}
}

// TestHistsyncRoutesParsed 钉住「确实读到了路由」。
//
// 上面那个测试在解析失效时会空跑通过;这里用一条已知路由当哨兵,
// 保证读到的是真路由表而不是空集。
func TestHistsyncRoutesParsed(t *testing.T) {
	routes := registeredHistsyncRoutes(t)

	const sentinel = "get /api/v1/histsync/channels"
	if _, ok := routes[sentinel]; !ok {
		t.Fatalf("路由表里没有哨兵路由 %q —— 读到的路由表不对,门禁失效。实际读到 %d 条: %v",
			sentinel, len(routes), sortedKeys(routes))
	}
	if len(routes) < 20 {
		t.Fatalf("只读到 %d 条 histsync 路由,数量明显偏少(预期 20+)。实际集合: %v",
			len(routes), sortedKeys(routes))
	}
}

// TestParseHistsyncRoutes 用一份合成的路由源文件验证解析器本身:
//
//   - 能还原嵌套分组的完整前缀(不是只看最后一段);
//   - 新加的路由一定会被读出来 —— 这正是「有人加了路由却忘了更新契约」能被抓住的前提;
//   - 非 histsync 的分组不会混进来。
func TestParseHistsyncRoutes(t *testing.T) {
	src := `package git_sync

func Register(r *server.Hertz) {
	root := r.Group("/", rootMw()...)
	_api := root.Group("/api", _apiMw()...)
	_v1 := _api.Group("/v1", _v1Mw()...)
	_mirror := _v1.Group("/mirror")
	_mirror.GET("/channels", handlerA)
	_histsync := _v1.Group("/histsync")
	_histsync.GET("/channels", handlerB)
	_histsync.POST("/channels/:id/audit", handlerC)
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "router.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("写临时路由文件失败: %v", err)
	}

	routes, err := parseHistsyncRoutes(path)
	if err != nil {
		t.Fatalf("解析合成路由文件失败: %v", err)
	}

	want := map[string]bool{
		"get /api/v1/histsync/channels":             true,
		"post /api/v1/histsync/channels/{id}/audit": true,
	}
	if len(routes) != len(want) {
		t.Fatalf("解析到 %d 条路由,期望 %d 条: %v", len(routes), len(want), sortedKeys(routes))
	}
	for key := range want {
		if _, ok := routes[key]; !ok {
			t.Fatalf("没读到路由 %q,实际读到: %v", key, sortedKeys(routes))
		}
	}
	// mirror 分组不该混进 histsync。
	for key := range routes {
		if strings.Contains(key, "/mirror") {
			t.Fatalf("非 histsync 的路由混进来了: %q", key)
		}
	}
}

// readRepoFile 读仓库里的一个文件(相对 internal/pkg/swagger 目录)。
func readRepoFile(t *testing.T, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", rel, err)
	}
	return raw
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
