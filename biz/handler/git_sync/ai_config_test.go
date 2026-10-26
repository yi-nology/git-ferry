package git_sync

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/agent"
)

// newAIConfigEnv 起一个带指定角色的 AI 配置端点环境。
func newAIConfigEnv(t *testing.T, role Role) *hertzserver.Hertz {
	t.Helper()
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	return newAIConfigEnvWithStore(t, role, store)
}

// newAIConfigEnvWithStore 复用既有设置库起环境(用于「先 admin 存一次,再 operator 存第二次」)。
func newAIConfigEnvWithStore(t *testing.T, role Role, store *agent.SettingsStore) *hertzserver.Hertz {
	t.Helper()
	SetAISettingsStore(func() *agent.SettingsStore { return store })
	SetAIRebuildRunner(func(st *agent.Settings) error { return nil })

	h := hertzserver.Default()
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		SetAuthRole(c, role)
		c.Next(ctx)
	})
	h.POST("/api/v1/ai/config", AIUpdateConfig)
	return h
}

func postAIConfig(t *testing.T, h *hertzserver.Hertz, body string) *ut.ResponseRecorder {
	t.Helper()
	return ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ai/config",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
}

// TestAIUpdateConfig_只读角色不能改AI配置 AI 配置决定「用什么模型/什么端点/
// 什么凭据」,只读角色不该能改。
func TestAIUpdateConfig_只读角色不能改AI配置(t *testing.T) {
	h := newAIConfigEnv(t, RoleReadonly)
	w := postAIConfig(t, h, `{"enabled":true,"base_url":"http://x/v1","model":"m"}`)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "forbidden")
}

// TestAIUpdateConfig_换后端需要admin 换后端 = 让另一个程序(可能是跑在本机上的
// 外部 coding agent)接管 AI 助手 —— 这是治理动作,不是普通运维参数。
func TestAIUpdateConfig_换后端需要admin(t *testing.T) {
	h := newAIConfigEnv(t, RoleOperator)
	w := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode"}}`)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "admin")
}

// TestAIUpdateConfig_改授权档需要admin 外部 agent 的授权档决定它能做什么。
func TestAIUpdateConfig_改授权档需要admin(t *testing.T) {
	h := newAIConfigEnv(t, RoleOperator)
	w := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode","permission":"approve_all"}}`)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "admin")
}

// TestAIUpdateConfig_普通参数改动不需要admin 换模型/改端点只是普通运维参数,
// 不该被治理闸门误伤(否则 operator 改个模型都要找管理员)。
func TestAIUpdateConfig_普通参数改动不需要admin(t *testing.T) {
	h := newAIConfigEnv(t, RoleOperator)
	// 内置后端要模型密钥(外部后端才不需要)⇒ 这里带上 api_key
	w := postAIConfig(t, h, `{"enabled":true,"base_url":"http://x/v1","model":"m2","api_key":"sk-x"}`)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAIUpdateConfig_admin可换后端 admin 是唯一能换后端的角色。
func TestAIUpdateConfig_admin可换后端(t *testing.T) {
	h := newAIConfigEnv(t, RoleAdmin)
	w := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode"}}`)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAIUpdateConfig_未填授权档不算变更 「没填」与「显式填默认值」是同一件事,
// 不该被误判成治理动作变更(否则第一次保存就被 admin 闸门卡住)。
func TestAIUpdateConfig_未填授权档不算变更(t *testing.T) {
	// 第一次存:从默认切到 acpx = 治理变更 ⇒ 用 admin
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	hAdmin := newAIConfigEnvWithStore(t, RoleAdmin, store)
	w0 := postAIConfig(t, hAdmin, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode","permission":"ask"}}`)
	require.Equal(t, http.StatusOK, w0.Code)

	// 再存一次:后端不变、授权档不填 ⇒ 不算治理变更 ⇒ operator 也该放行
	h := newAIConfigEnvWithStore(t, RoleOperator, store)
	w1 := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode"}}`)
	assert.Equal(t, http.StatusOK, w1.Code,
		"未填授权档(等价默认 ask)不该被当成变更而要求 admin")
}
