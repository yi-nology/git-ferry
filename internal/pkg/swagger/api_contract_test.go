package swagger

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// 全 API 契约门禁:OpenAPI 与**实际注册的路由**必须**双向**一致。
//
// 之前只有 histsync / mirror 两个前缀有门禁(biz/router/git_sync/git_sync.go 里的
// 手写路由),ops / ai / platform / webhook 等其它路由没人管 —— 于是积了两类漂移:
//
//	① 路由有、spec 没有 ⇒ spec 驱动的客户端(swagger UI / `gitferry schema`)
//	   **看不见**这些接口;
//	② spec 有、路由没有 ⇒ **spec 在说谎**:调用方照契约写代码、照契约调用,
//	   实际得到 404/405。② 比 ① 更危险 —— 契约是给调用方看的,说谎等于给错地图。
//
// 门禁双向都查,任何一侧漂移都红。
const contractScope = "/api/v1"

// routerSources 返回全部路由注册源文件(绝对路径)。
//
// **用 glob 而不是写死两个文件**:新增第三个注册文件时,写死的列表会读不到它 ⇒
// 那个文件里"已注册但契约没有"的接口**两个方向都测不到**
// (解析器看不见 ⇒ 不算 missing;契约没声明 ⇒ 不算 ghost)
// ⇒ 门禁静默失去覆盖,而注释还宣称"全 API 契约门禁"。
func routerSources(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	var out []string
	err := filepath.WalkDir(filepath.Join(root, "biz", "router"), func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d == nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("扫描路由源文件失败: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("没找到任何路由源文件(目录布局变了?)")
	}
	return out
}

// registeredContractRoutes 返回**实际注册**且属于契约范围的路由集合。
//
// 键是 "<method> <path>"(小写方法名 + OpenAPI 风格路径),值是 handler 名。
func registeredContractRoutes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, src := range routerSources(t) {
		routes, err := parseRoutesByPrefix(src, contractScope)
		if err != nil {
			t.Fatalf("解析路由源文件 %s 失败: %v", src, err)
		}
		for k, v := range routes {
			out[k] = v
		}
	}
	if len(out) == 0 {
		t.Fatalf("没从路由源文件读到任何 %s 路由 —— 路由注册方式变了?本门禁会失去意义", contractScope)
	}
	return out
}

// TestContractRoutesParsed 哨兵:确保解析器真的读到了路由
// (否则下面的门禁会因为空集而空跑通过)。
func TestContractRoutesParsed(t *testing.T) {
	routes := registeredContractRoutes(t)
	for _, sentinel := range []string{
		"get /api/v1/repos",
		"post /api/v1/sync/task/run",
		"post /api/v1/ops/force-push-approvals/approve",
	} {
		if _, ok := routes[sentinel]; !ok {
			t.Fatalf("解析器没读到哨兵路由 %q(读到的前几条:%v)", sentinel, firstKeys(routes, 5))
		}
	}
}

// TestOpenAPICoversRegisteredRoutes 已注册的路由**必须**出现在 OpenAPI 里。
func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	routes := registeredContractRoutes(t)
	for _, spec := range contractSpecs(t) {
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
				t.Fatalf("OpenAPI 未覆盖以下已注册的路由:\n  %s\n"+
					"契约补齐后跑 `make apidoc` 重新生成。",
					strings.Join(missing, "\n  "))
			}
		})
	}
}

// TestOpenAPIDoesNotLie OpenAPI 里声明的 (方法,路径) **必须真的注册了**。
//
// 这一侧以前没人查 —— 于是出现了「spec 说 GET、实际注册的是 POST」这类漂移:
// 调用方照契约发 GET,实际 404/405,而契约本身还写得煞有介事。
func TestOpenAPIDoesNotLie(t *testing.T) {
	routes := registeredContractRoutes(t)
	for _, spec := range contractSpecs(t) {
		t.Run(spec.name, func(t *testing.T) {
			paths := openAPIPaths(t, spec.raw)
			var ghosts []string
			for path, methods := range paths {
				if !strings.HasPrefix(path, contractScope) {
					continue
				}
				for method := range methods {
					if _, ok := routes[method+" "+path]; !ok {
						ghosts = append(ghosts, strings.ToUpper(method)+" "+path+" (spec 声明了但没有注册)")
					}
				}
			}
			if len(ghosts) > 0 {
				sort.Strings(ghosts)
				t.Fatalf("OpenAPI 声明了以下**未注册**的端点(契约说谎 ⇒ 调用方按契约调用会 404/405):\n  %s\n"+
					"要么改路由注册,要么改契约,再跑 `make apidoc` 重新生成。",
					strings.Join(ghosts, "\n  "))
			}
		})
	}
}

// contractSpecs 返回要校验的契约副本(内嵌的那份 + 仓里 docs 那份)。
func contractSpecs(t *testing.T) []struct {
	name string
	raw  []byte
} {
	t.Helper()
	return []struct {
		name string
		raw  []byte
	}{
		{"internal/pkg/swagger/openapi.json", Spec()},
		{"docs/openapi.json", readRepoFile(t, filepath.Join(repoRoot(t), "docs", "openapi.json"))},
	}
}

// firstKeys 取 map 的前 n 个键(错误信息用)。
func firstKeys(m map[string]string, n int) []string {
	keys := sortedKeys(m)
	if len(keys) > n {
		keys = keys[:n]
	}
	return keys
}
