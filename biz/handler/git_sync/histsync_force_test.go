package git_sync

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
)

// TestHistoryPushForceRequiresAdmin 回归:强制推送要 admin 角色。
//
// 强推会覆盖远端已有的提交(别人的提交被静默丢掉就是后果),
// 平台在 ops 侧对同类破坏性动作做了「申请 + admin 批准」两段式,
// 这里不该只要 operator。
func TestHistoryPushForceRequiresAdmin(t *testing.T) {
	for _, tc := range []struct {
		role   Role
		denied bool
	}{
		{RoleReadonly, true},
		{RoleOperator, true},
		{RoleAdmin, false},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			h := hertzserver.Default()
			// 鉴权中间件做的事:把角色写进请求上下文。
			h.Use(func(ctx context.Context, c *app.RequestContext) {
				SetAuthRole(c, tc.role)
				c.Next(ctx)
			})
			h.POST("/api/v1/histsync/channels/:id/push", HistoryChannelPush)

			// **字段名是 `force_push`** —— 与 `/sync` 端点的语义归一后,
			// `/push` 不再接受 `force`(旧名会被明确拒绝,见下面的用例)。
			body := `{"force_push":true}`
			w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/histsync/channels/999/push",
				&ut.Body{Body: strings.NewReader(body), Len: len(body)},
				ut.Header{Key: "Content-Type", Value: "application/json"})

			if tc.denied {
				assert.Contains(t, w.Body.String(), "forbidden",
					"角色 %s 不应能强制推送", tc.role)
				return
			}
			// admin:不该被角色挡住(后续会因为通道不存在而报错,那是另一回事)。
			assert.NotContains(t, w.Body.String(), "forbidden",
				"admin 应能强制推送")
		})
	}
}

// TestHistoryPushRejectsLegacyForceKey 回归:旧字段名 `force` 必须**明确报错**。
//
// 同一个键 `force` 在 `/sync` 表示「忽略阻塞项继续」,在 `/push` 却是
// 「强制推送」—— 语义相反的同名键迟早被用错。现在 `/push` 只认 `force_push`。
//
// **为什么不能静默忽略**:旧调用方发 `{"force":true}` 会被当成「没传」——
// 用户以为强推了,实际没推(而强推是「覆盖远端已有提交」这种破坏性动作,
// 误以为推了比推不动更危险)。
func TestHistoryPushRejectsLegacyForceKey(t *testing.T) {
	h := hertzserver.Default()
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		SetAuthRole(c, RoleAdmin)
		c.Next(ctx)
	})
	h.POST("/api/v1/histsync/channels/:id/push", HistoryChannelPush)

	body := `{"force":true}`
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/histsync/channels/999/push",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})

	assert.Contains(t, w.Body.String(), "force_push",
		"旧字段名必须明确报错并指路:%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "forbidden",
		"旧字段名是 400(指路),不是角色问题")
}
