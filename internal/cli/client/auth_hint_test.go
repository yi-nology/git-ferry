package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDo_401_本机无密钥时提示要说准 本机**根本没拿到**密钥时,401 的提示
// 必须说清"不是密钥不对,是这台机器上没拿到密钥"。
//
// 回归背景(P1,把人引向错误的修法):密钥存在 macOS Keychain 时,配置文件里
// 的 api_key 是空的 ⇒ 非交互环境读不到 Keychain ⇒ CLI 不带任何认证发请求
// ⇒ 服务端 401 ⇒ 提示却是通用的「请设置 GITFERRY_TOKEN 或运行
// gitferry auth login」⇒ 用户以为自己没配过,反复重新设置 token,
// 而真正的解法是"这个环境读不到 Keychain 里的密钥"。
//
// 这条用例是**反向验证**:去掉客户端的 401 提示改写,本用例会变红。
func TestDo_401_本机无密钥时提示要说准(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-API-Key"), "本机没有密钥时不该发认证头")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "") // 本机没有可用密钥
	env, err := c.Get("/api/v1/system/status", nil)
	require.NoError(t, err)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Suggestion, "本机没有可用的 API Key",
		"提示必须点明「本机没拿到密钥」,而不是让用户去重新设置 token")
	assert.Contains(t, env.Error.Suggestion, "Keychain")
}

// TestDo_401_有密钥时保留通用提示 本机**有**密钥却被拒 ⇒ 就是密钥不对 /
// 权限不足 ⇒ 不该被改写成"本机没有密钥"。
func TestDo_401_有密钥时保留通用提示(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my-key", r.Header.Get("X-API-Key"))
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "my-key")
	env, err := c.Get("/api/v1/system/status", nil)
	require.NoError(t, err)
	require.NotNil(t, env.Error)
	assert.NotContains(t, env.Error.Suggestion, "本机没有可用的 API Key",
		"有密钥时不该说「本机没有密钥」")
	assert.Contains(t, env.Error.Suggestion, "GITFERRY_TOKEN")
}

// TestDo_非401错误不受影响 其它状态的提示不能被这次改动波及。
func TestDo_非401错误不受影响(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	env, err := c.Get("/api/v1/system/status", nil)
	require.NoError(t, err)
	require.NotNil(t, env.Error)
	assert.NotContains(t, env.Error.Suggestion, "本机没有可用的 API Key")
	assert.Contains(t, env.Error.Suggestion, "角色权限不足")
}
