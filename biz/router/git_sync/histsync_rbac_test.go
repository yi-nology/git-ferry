package git_sync

import (
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"

	handler "github.com/yi-nology/git-ferry/biz/handler/git_sync"
)

// histsyncEngine 注册全部路由(含历史同步),并把 API Key 角色设成指定角色。
//
// API Key 与角色都是**全局状态**,用完必须还原 ——
// 否则同包里的其它用例会被带偏(既有的鉴权用例就依赖它)。
func histsyncEngine(t *testing.T, role string) *server.Hertz {
	t.Helper()
	oldKey, oldRole := handler.GetAPIKey(), handler.GetAPIKeyRole()
	handler.SetAPIKey(testAPIKey)
	handler.SetAPIKeyRole(role)
	t.Cleanup(func() {
		handler.SetAPIKey(oldKey)
		handler.SetAPIKeyRole(oldRole)
	})
	h := server.Default()
	Register(h)
	return h
}

func histsyncRequest(t *testing.T, h *server.Hertz, method, path string) int {
	t.Helper()
	w := ut.PerformRequest(h.Engine, method, path, nil,
		ut.Header{Key: "X-API-Key", Value: testAPIKey})
	return w.Code
}

// TestHistsyncWriteRoutesRequireWriteRole 回归:历史同步的写操作必须要求
// 读写角色。
//
// 背景:这些路由原本**一个守卫都没有** —— OIDC 的默认角色 readonly
// 也能建通道、改推送目标、强推、回滚。推送目标是调用方指定的,
// 于是「只读用户把内容推到攻击者的远端」是可行的。
func TestHistsyncWriteRoutesRequireWriteRole(t *testing.T) {
	h := histsyncEngine(t, "readonly")

	writeRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/histsync/channels"},
		{http.MethodPost, "/api/v1/histsync/channels/1/update"},
		{http.MethodPost, "/api/v1/histsync/channels/1/delete"},
		{http.MethodPost, "/api/v1/histsync/channels/1/targets"},
		{http.MethodPost, "/api/v1/histsync/targets/1/update"},
		{http.MethodPost, "/api/v1/histsync/targets/1/delete"},
		{http.MethodPost, "/api/v1/histsync/targets/1/rotate"},
		{http.MethodPost, "/api/v1/histsync/channels/1/apply"},
		{http.MethodPost, "/api/v1/histsync/channels/1/push"},
		{http.MethodPost, "/api/v1/histsync/channels/1/sync"},
		{http.MethodPost, "/api/v1/histsync/channels/1/overrides"},
		{http.MethodPost, "/api/v1/histsync/overrides/1/delete"},
	}
	for _, r := range writeRoutes {
		code := histsyncRequest(t, h, r.method, r.path)
		assert.Equal(t, http.StatusForbidden, code,
			"只读角色不应能访问 %s %s", r.method, r.path)
	}

	// 回滚是强推、覆盖已发布内容 —— 只读角色更不能碰。
	assert.Equal(t, http.StatusForbidden,
		histsyncRequest(t, h, http.MethodPost, "/api/v1/histsync/runs/1/rollback"),
		"只读角色不应能回滚")
}

// TestHistsyncReadRoutesAllowReadonly 读操作对只读角色放开。
func TestHistsyncReadRoutesAllowReadonly(t *testing.T) {
	h := histsyncEngine(t, "readonly")
	for _, path := range []string{
		"/api/v1/histsync/channels",
		"/api/v1/histsync/channels/1",
		"/api/v1/histsync/channels/1/targets",
		"/api/v1/histsync/channels/1/runs",
		"/api/v1/histsync/channels/1/audit",
		"/api/v1/histsync/channels/1/overrides",
		"/api/v1/histsync/runs/1",
	} {
		code := histsyncRequest(t, h, http.MethodGet, path)
		assert.NotEqual(t, http.StatusForbidden, code,
			"只读角色应能读 %s", path)
	}
}

// TestHistsyncRollbackRequiresAdmin 回滚要 admin 角色:它强推并覆盖已发布内容。
func TestHistsyncRollbackRequiresAdmin(t *testing.T) {
	h := histsyncEngine(t, "operator")
	code := histsyncRequest(t, h, http.MethodPost, "/api/v1/histsync/runs/1/rollback")
	assert.Equal(t, http.StatusForbidden, code,
		"operator 角色不应能回滚(强推、覆盖已发布内容)")
}

// TestHistsyncWriteRoutesAllowOperator 读写角色能写(但不能回滚)。
func TestHistsyncWriteRoutesAllowOperator(t *testing.T) {
	h := histsyncEngine(t, "operator")
	code := histsyncRequest(t, h, http.MethodPost, "/api/v1/histsync/channels")
	assert.NotEqual(t, http.StatusForbidden, code,
		"operator 角色应能建通道")
}
