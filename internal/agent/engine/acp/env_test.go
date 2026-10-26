package acp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScrubEnv_摘掉本服务的密钥 外部 agent 子进程默认继承父进程环境 ⇒
// 它能读到 ENCRYPTION_KEY(凭据加密主密钥)与 admin 主 key,
// 绕开 MCP 的只读边界直��调写接口。
func TestScrubEnv_摘掉本服务的密钥(t *testing.T) {
	in := []string{
		"ENCRYPTION_KEY=deadbeef",
		"GIT_SYNC_SERVER_API_KEY=adminkey",
		"GIT_SYNC_AI_API_KEY=modelkey",
		"GITFERRY_AI_MCP_API_KEY=mcprokey",
		"PATH=/usr/bin:/bin",
		"HOME=/Users/someone",
		"ANTHROPIC_API_KEY=provider-key", // agent 自己的 provider 鉴权要留
		"LANG=zh_CN.UTF-8",
	}
	got := scrubEnv(in)
	joined := strings.Join(got, " ")

	// 本服务的密钥必须没了
	assert.NotContains(t, joined, "ENCRYPTION_KEY")
	assert.NotContains(t, joined, "GIT_SYNC_SERVER_API_KEY")
	assert.NotContains(t, joined, "GIT_SYNC_AI_API_KEY")
	assert.NotContains(t, joined, "GITFERRY_AI_MCP_API_KEY")

	// 其余必须留(否则 agent 跑不起来)
	assert.Contains(t, joined, "PATH=/usr/bin:/bin")
	assert.Contains(t, joined, "HOME=/Users/someone")
	assert.Contains(t, joined, "ANTHROPIC_API_KEY=provider-key")
	assert.Contains(t, joined, "LANG=zh_CN.UTF-8")
	assert.Len(t, got, 4)
}

// TestScrubEnv_不误伤相似名字 只按精确名与前缀摘,别把无关变量也清了。
func TestScrubEnv_不误伤相似名字(t *testing.T) {
	in := []string{
		"MY_ENCRYPTION_KEY_HINT=keepme", // 不是 ENCRYPTION_KEY ⇒ 留
		"GIT_SYNCX=keepme",              // 不是 GIT_SYNC_ 前缀 ⇒ 留
		"GITFERRY_TOOL=keepme",          // 本仓命名空间 ⇒ 摘
	}
	got := scrubEnv(in)
	joined := strings.Join(got, " ")
	assert.Contains(t, joined, "MY_ENCRYPTION_KEY_HINT=keepme")
	assert.Contains(t, joined, "GIT_SYNCX=keepme")
	assert.NotContains(t, joined, "GITFERRY_TOOL", "本仓命名空间(GITFERRY_*)一律摘掉")
	assert.Len(t, got, 2)
}
