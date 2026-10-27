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
//
// 注意:acpx 后端的授权档必须是 approve_reads|approve_all|deny ——
// 「ask」在非交互通道上名不副实,保存时就会被拒(见 agent 包的 Save 校验)。
func TestAIUpdateConfig_admin可换后端(t *testing.T) {
	h := newAIConfigEnv(t, RoleAdmin)
	w := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode","permission":"approve_reads"}}`)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAIUpdateConfig_acpx用ask档被拒 acpx 非交互,弹不出授权卡 ⇒ 直接拒。
func TestAIUpdateConfig_acpx用ask档被拒(t *testing.T) {
	h := newAIConfigEnv(t, RoleAdmin)
	w := postAIConfig(t, h, `{"enabled":true,"backend":"acpx","external":{"agent":"mcode","permission":"ask"}}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "非交互")
}

// TestAIUpdateConfig_未填授权档不算变更 「没填」与「显式填默认值」是同一件事,
// 不该被误判成治理动作变更(否则第一次保存就被 admin 闸门卡住)。
func TestAIUpdateConfig_未填授权档不算变更(t *testing.T) {
	// 用 **stdio** 后端(它才支持逐次确认 ⇒ ask 是合法档位)
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	hAdmin := newAIConfigEnvWithStore(t, RoleAdmin, store)
	w0 := postAIConfig(t, hAdmin, `{"enabled":true,"backend":"stdio","external":{"command":"mcode acp","permission":"ask"}}`)
	require.Equal(t, http.StatusOK, w0.Code)

	// 再存一次:后端不变、授权档不填 ⇒ 不算治理变更 ⇒ operator 也该放行
	h := newAIConfigEnvWithStore(t, RoleOperator, store)
	w1 := postAIConfig(t, h, `{"enabled":true,"backend":"stdio","external":{"command":"mcode acp"}}`)
	assert.Equal(t, http.StatusOK, w1.Code,
		"未填授权档(等价默认 ask)不该被当成变更而要求 admin")
}

// TestAIUpdateConfig_改profile授权档需要admin profile 列表模式下,改**某个
// profile** 的授权档同样是治理动作 —— 只比单字段的话会被漏过去
// (授权档决定外部 agent 能读什么/写什么 ⇒ 这是安全边界)。
func TestAIUpdateConfig_改profile授权档需要admin(t *testing.T) {
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	hAdmin := newAIConfigEnvWithStore(t, RoleAdmin, store)
	w0 := postAIConfig(t, hAdmin,
		`{"enabled":true,"backend":"stdio","external":{"agents":[{"name":"mcode",`+
			`"transport":"stdio","command":"mcode acp","permission":"ask"}]}}`)
	require.Equal(t, http.StatusOK, w0.Code)

	// 换成 operator,只改 profile 的授权档 ⇒ 仍是治理动作 ⇒ 403
	h := newAIConfigEnvWithStore(t, RoleOperator, store)
	w1 := postAIConfig(t, h,
		`{"enabled":true,"backend":"stdio","external":{"agents":[{"name":"mcode",`+
			`"transport":"stdio","command":"mcode acp","permission":"approve_all"}]}}`)
	assert.Equal(t, http.StatusForbidden, w1.Code)
	assert.Contains(t, w1.Body.String(), "admin")
}

// TestAIGetConfig_回生效配置 设置文件只是**覆盖层**,
// 真正生效的是「yaml 默认 + 设置文件覆盖」。
// 只回设置文件的话,用 yaml 配置的部署在界面上看到的是**空表单**
// (得把配置全部重敲一遍才能保存)。
func TestAIGetConfig_回生效配置(t *testing.T) {
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	SetAISettingsStore(func() *agent.SettingsStore { return store })
	SetAIYamlConfig(&agent.Config{
		Enabled: true,
		Backend: agent.BackendInternal,
		BaseURL: "http://yaml-endpoint/v1",
		Model:   "yaml-model",
	})
	t.Cleanup(func() { SetAIYamlConfig(nil) })

	h := hertzserver.Default()
	h.GET("/api/v1/ai/config", AIGetConfig)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ai/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "yaml-model", "应回 yaml 里的生效值")
	assert.Contains(t, w.Body.String(), "http://yaml-endpoint/v1")
}

// TestAIGetConfig_设置文件覆盖yaml 设置文件里的值优先于 yaml。
func TestAIGetConfig_设置文件覆盖yaml(t *testing.T) {
	store, err := agent.OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)
	_, err = store.Save(&agent.Settings{
		Enabled: true,
		Backend: agent.BackendACPX,
		Model:   "from-file",
		External: agent.ExternalSettings{
			Agent:      "mcode",
			Permission: "approve_reads",
		},
	})
	require.NoError(t, err)
	SetAISettingsStore(func() *agent.SettingsStore { return store })
	SetAIYamlConfig(&agent.Config{Enabled: true, Backend: agent.BackendInternal, Model: "from-yaml"})
	t.Cleanup(func() { SetAIYamlConfig(nil) })

	h := hertzserver.Default()
	h.GET("/api/v1/ai/config", AIGetConfig)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ai/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "from-file", "设置文件应覆盖 yaml")
}
