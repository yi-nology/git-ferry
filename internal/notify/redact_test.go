package notify

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedactSecretURL_URL 里的秘密不能原样出现在日志里。
//
// 回归背景(P1):Gotify 的 token 拼在 **query** 里 ⇒ http.Client 返回的
// *url.Error 的 Error() 含**完整 URL** ⇒ 错误进日志 ⇒ 令牌落到日志系统。
// healthchecks.io 的 ping UUID 在**路径**里(同样是秘密)。
func TestRedactSecretURL_URL(t *testing.T) {
	// query 里的 token
	got := redactSecretURL("http://gotify.local/message?token=GOTIFY-SECRET&title=x")
	assert.NotContains(t, got, "GOTIFY-SECRET", "query 里的 token 必须被脱敏")
	assert.Contains(t, got, "REDACTED")

	// 路径里的 UUID(healthchecks)
	got2 := redactSecretURL("https://hc-ping.com/ping/UUID-1234-5678/fail")
	assert.NotContains(t, got2, "UUID-1234-5678", "路径里的 UUID 必须被脱敏")
	assert.Contains(t, got2, "hc-ping.com", "host 要保留(排障要认得出是哪个目标)")

	// 非秘密参数保留
	got3 := redactSecretURL("http://example.com/x?page=2")
	assert.Contains(t, got3, "page=2")

	// 解析不了的不给原文
	assert.Equal(t, "<unparsable-url>", redactSecretURL("://bad url"))
}

// TestRedactError_UrlError 错误信息里的 URL 必须脱敏。
func TestRedactError_UrlError(t *testing.T) {
	inner := &url.Error{
		Op:  "Post",
		URL: "http://gotify.local/message?token=GOTIFY-SECRET",
		Err: assertErr{},
	}
	got := redactError(inner)
	require.NotNil(t, got)
	assert.NotContains(t, got.Error(), "GOTIFY-SECRET",
		"*url.Error 的消息含完整 URL ⇒ 错误信息必须脱敏")
	assert.Contains(t, got.Error(), "gotify.local", "host 要保留")

	// nil 原样返回
	assert.Nil(t, redactError(nil))
}

// TestIsSecretParam 秘密参数名要覆盖常见几种。
func TestIsSecretParam(t *testing.T) {
	for _, k := range []string{"token", "TOKEN", "access_token", "private_token", "password", "api_key", "apikey"} {
		assert.True(t, isSecretParam(k), "应判定为秘密参数: %s", k)
	}
	for _, k := range []string{"page", "title", "priority", "tags"} {
		assert.False(t, isSecretParam(k), "不应判定为秘密参数: %s", k)
	}
}

// assertErr 一个最小的 error 实现。
type assertErr struct{}

func (assertErr) Error() string { return "boom" }

// TestRedactSecretURL_KeepsHost 确认脱敏后仍能定位目标。
func TestRedactSecretURL_KeepsHost(t *testing.T) {
	got := redactSecretURL("https://notify.example.com/v1/hook?token=abc")
	assert.True(t, strings.HasPrefix(got, "https://notify.example.com"), "host 必须保留: %s", got)
}

// TestNotifyRun_ErrorDoesNotLeakToken 通知失败时返回的错误里**不得**含令牌。
//
// 回归背景(P1):Gotify 的 token 拼在 query 里 ⇒ 请求失败时
// http.Client 返回 *url.Error,其 Error() 含**完整 URL** ⇒ 令牌随错误进日志。
//
// 这条用例走**真实路径**(Notifier → Gotify → 失败),所以它是行为级的:
// 去掉 do() 里的脱敏,本用例会变红。
func TestNotifyRun_ErrorDoesNotLeakToken(t *testing.T) {
	// 指向一个不存在的端口 ⇒ 必然连接失败 ⇒ 错误里会带完整 URL
	n := New(&Config{
		Gotify: []GotifyConfig{{URL: "http://127.0.0.1:1", Token: "GOTIFY-SECRET-TOKEN"}},
	})
	err := n.sendGotify(context.Background(), &GotifyConfig{
		URL: "http://127.0.0.1:1", Token: "GOTIFY-SECRET-TOKEN",
	}, &RunEvent{TaskName: "T", Status: "failed"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "GOTIFY-SECRET-TOKEN",
		"失败错误里不得含令牌(会进日志)")
}
