package swagger

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mirrorPrefix 是镜像中心路由的公共前缀。
const mirrorPrefix = "/api/v1/mirror"

// TestMirrorOpenAPICoversRegisteredRoutes 镜像中心的**防漂移门禁**。
//
// 断言:OpenAPI 里的 paths 集合**覆盖**实际注册的 mirror 路由集合(方法 + 路径)。
// 以后有人加/改 mirror 路由(biz/router/git_sync/git_sync.go)却忘了更新契约
// (cmd/apidoc/mirror.go),这里就会红。
//
// 与 histsync 的门禁同款(同一个解析器,只换前缀)。
func TestMirrorOpenAPICoversRegisteredRoutes(t *testing.T) {
	routes, err := parseRoutesByPrefix(routerSource(t), mirrorPrefix)
	if err != nil {
		t.Fatalf("解析路由源文件 %s 失败: %v", routerSource(t), err)
	}
	if len(routes) == 0 {
		t.Fatalf("没从 %s 读到任何 mirror 路由 —— 路由注册方式变了?本门禁会失去意义", routerSource(t))
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
				t.Fatalf("OpenAPI 未覆盖以下已注册的 mirror 路由:\n  %s\n"+
					"契约补齐后跑 `make apidoc` 重新生成。",
					strings.Join(missing, "\n  "))
			}
		})
	}
}

// TestMirrorRoutesParsed 钉住「确实读到了路由」——
// 上面的测试在解析失效时会空跑通过;这里用一条已知路由当哨兵。
func TestMirrorRoutesParsed(t *testing.T) {
	routes, err := parseRoutesByPrefix(routerSource(t), mirrorPrefix)
	if err != nil {
		t.Fatalf("解析路由源文件失败: %v", err)
	}
	const sentinel = "get /api/v1/mirror/channels"
	if _, ok := routes[sentinel]; !ok {
		t.Fatalf("解析器没读到哨兵路由 %q(读到的:%v)", sentinel, sortedKeys(routes))
	}
}
