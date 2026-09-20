// Package swagger 内嵌 OpenAPI 文档与 Swagger UI,服务启动即可浏览 API 文档。
// 路由:GET /swagger/ (UI)、GET /swagger/openapi.json (spec)。
// spec 文件由 `go run ./cmd/apidoc` 从 idl/*.thrift 生成,`make apidoc` 同步更新。
package swagger

import (
	"context"
	"embed"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

//go:embed assets/index.html
var indexFS embed.FS

//go:embed openapi.json
var specFS embed.FS

// SwaggerUI 渲染 Swagger UI 页面。
func SwaggerUI(ctx context.Context, c *app.RequestContext) {
	data, _ := indexFS.ReadFile("assets/index.html")
	c.Data(consts.StatusOK, "text/html; charset=utf-8", data)
}

// OpenAPISpec 返回 OpenAPI 3.0 JSON spec。
func OpenAPISpec(ctx context.Context, c *app.RequestContext) {
	data, _ := specFS.ReadFile("openapi.json")
	c.Data(consts.StatusOK, "application/json; charset=utf-8", data)
}
