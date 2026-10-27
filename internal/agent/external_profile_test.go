package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidate_profile模式下不再误报 **踩过的坑**:校验查的是旧的单字段
// (`external.command`),而 profile 模式下命令写在 profile 里
// ⇒ 校验失败 ⇒ 启动直接报错 ⇒ AI 根本没起来(端点回落 501)。
//
// 这个测试把「profile 模式下 Validate 要过」钉死。
func TestValidate_profile模式下不再误报(t *testing.T) {
	c := &Config{
		Enabled: true,
		Backend: BackendStdio,
		External: ExternalConfig{
			Agents: []ExternalAgentConfig{
				{Name: "mcode-acp", Transport: BackendStdio, Command: "mcode acp", Permission: "approve_reads"},
			},
			Current: "mcode-acp",
		},
	}
	require.NoError(t, c.Validate(""), "profile 模式下不该报「需要 external.command」")

	// 单 profile 字段没填也不该影响(profile 列表为准)。
	assert.Empty(t, c.External.Command)

	// 少填 command 就要报错 —— 且要指出是哪个 profile。
	bad := &Config{
		Enabled: true,
		Backend: BackendStdio,
		External: ExternalConfig{
			Agents: []ExternalAgentConfig{{Name: "broken", Transport: BackendStdio}},
		},
	}
	err := bad.Validate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
}

// TestEffectiveExternal_取生效profile 生效的 profile = Current 指定的;
// 没指定就是列表第一个。传输方式跟随 profile 自己的声明。
func TestEffectiveExternal_取生效profile(t *testing.T) {
	c := &Config{
		Backend: BackendACPX,
		External: ExternalConfig{
			Agents: []ExternalAgentConfig{
				{Name: "claude", Transport: BackendStdio, Command: "claude-code-acp"},
				{Name: "codex", Transport: BackendACPX, Agent: "codex"},
			},
			Current: "codex",
		},
	}
	p, transport := c.EffectiveExternal()
	assert.Equal(t, "codex", p.Name)
	assert.Equal(t, BackendACPX, transport)

	// 没写 Current ⇒ 第一个。
	c.External.Current = ""
	p, _ = c.EffectiveExternal()
	assert.Equal(t, "claude", p.Name)
}

// TestEffectiveExternal_internal后端不被profile顶掉 主后端是 internal 时,
// profile 列表只服务**委派**;profile 自己的 transport 不能把主后端顶成外部后端。
func TestEffectiveExternal_internal后端不被profile顶掉(t *testing.T) {
	c := &Config{
		Backend: BackendInternal,
		External: ExternalConfig{
			Agents: []ExternalAgentConfig{
				{Name: "codex", Transport: BackendACPX, Agent: "codex"},
			},
			Current: "codex",
		},
	}
	_, transport := c.EffectiveExternal()
	assert.Equal(t, BackendInternal, transport, "主后端不该被 profile 的 transport 顶掉")
}

// TestProfileTransportOf_按字段形状推断 老写法不带 transport:
// 有 command ⇒ stdio(命令就是 ACP agent 可执行文件);有 agent 名 ⇒ acpx。
func TestProfileTransportOf_按字段形状推断(t *testing.T) {
	// 主后端 internal + 只有 agent 名 ⇒ 委派走 acpx。
	assert.Equal(t, BackendACPX, ProfileTransportOf(BackendInternal,
		&ExternalAgentConfig{Agent: "mcode"}))
	// 主后端 internal + 只有 command ⇒ 委派走 stdio。
	assert.Equal(t, BackendStdio, ProfileTransportOf(BackendInternal,
		&ExternalAgentConfig{Command: "mcode acp"}))
	// 显式写的 transport 优先。
	assert.Equal(t, BackendStdio, ProfileTransportOf(BackendACPX,
		&ExternalAgentConfig{Transport: BackendStdio, Agent: "codex"}))
	// 都没写 ⇒ 跟随主后端。
	assert.Equal(t, BackendACPX, ProfileTransportOf(BackendACPX, &ExternalAgentConfig{}))
}

// TestSameExternalPermissions_profile改档要能看出来 治理闸门靠它判断
// 「授权档有没有变」—— 只比单字段的话,profile 列表模式下改授权档会被漏过去。
func TestSameExternalPermissions_profile改档要能看出来(t *testing.T) {
	base := ExternalConfig{
		Permission: "approve_reads",
		Agents: []ExternalAgentConfig{
			{Name: "codex", Permission: "approve_reads"},
		},
	}
	assert.True(t, SameExternalPermissions(&base, &base))

	// 改了某个 profile 的授权档 ⇒ 算变了。
	changed := base
	changed.Agents = []ExternalAgentConfig{{Name: "codex", Permission: "approve_all"}}
	assert.False(t, SameExternalPermissions(&base, &changed))

	// 单字段变了也算。
	single := base
	single.Permission = "approve_all"
	assert.False(t, SameExternalPermissions(&base, &single))

	// 空值与默认值等价(不算变)。
	withAsk := ExternalConfig{
		Permission: "ask",
		Agents:     []ExternalAgentConfig{{Name: "codex", Permission: "ask"}},
	}
	withBlank := ExternalConfig{
		Agents: []ExternalAgentConfig{{Name: "codex"}},
	}
	assert.True(t, SameExternalPermissions(&withAsk, &withBlank), "空值应等价于默认档 ask")
}
