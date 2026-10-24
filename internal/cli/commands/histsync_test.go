package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/cmdutil"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// histsyncSubCmd 取出历史同步命令组里的某个子命令。
func histsyncSubCmd(t *testing.T, name string) *cobra.Command {
	t.Helper()
	root := newHistsyncCmd()
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("找不到子命令 %s", name)
	return nil
}

// TestHistsyncFlagToBody 钉住 CLI 的 flag → 请求体映射。
//
// flag 名与 body key 拼错**不会编译报错**,只会静默不生效
// (比如把 target_branch 写成 target_ref)—— 没有这类测试就永远发现不了。
func TestHistsyncFlagToBody(t *testing.T) {
	type captured struct {
		path string
		body map[string]any
	}
	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{}}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "test-key")
	// 危险操作要放行,否则命令在发请求前就被确认闸拦下。
	oldYes := cmdutil.Yes
	cmdutil.Yes = true
	defer func() { cmdutil.Yes = oldYes }()

	t.Run("channel-create", func(t *testing.T) {
		cmd := histsyncSubCmd(t, "+channel-create")
		require.NoError(t, cmd.Flags().Set("name", "c1"))
		require.NoError(t, cmd.Flags().Set("module-from", "git.enjoye.top/a"))
		require.NoError(t, cmd.Flags().Set("module-to", "github.com/a"))
		require.NoError(t, cmd.Flags().Set("target-ref", "main"))
		_, err := histsyncChannelCreate(c, &config.Config{}, cmd, nil)
		require.NoError(t, err)

		assert.Equal(t, "/api/v1/histsync/channels", got.path)
		assert.Equal(t, "c1", got.body["name"])
		assert.Equal(t, "git.enjoye.top/a", got.body["module_source"])
		assert.Equal(t, "github.com/a", got.body["module_target"])
		assert.Equal(t, "main", got.body["target_branch"])
	})

	t.Run("push", func(t *testing.T) {
		cmd := histsyncSubCmd(t, "+push")
		require.NoError(t, cmd.Flags().Set("channel-id", "7"))
		require.NoError(t, cmd.Flags().Set("force-push", "true"))
		_, err := histsyncPush(c, &config.Config{}, cmd, nil)
		require.NoError(t, err)

		assert.Equal(t, "/api/v1/histsync/channels/7/push", got.path)
		assert.Equal(t, true, got.body["force_push"],
			"--force-push 必须映射成请求体的 force_push")
	})

	t.Run("override-set", func(t *testing.T) {
		cmd := histsyncSubCmd(t, "+override-set")
		require.NoError(t, cmd.Flags().Set("channel-id", "7"))
		require.NoError(t, cmd.Flags().Set("path", "docs/a.md"))
		require.NoError(t, cmd.Flags().Set("verdict", "keep_target"))
		_, err := histsyncOverrideSet(c, &config.Config{}, cmd, nil)
		require.NoError(t, err)

		assert.Equal(t, "/api/v1/histsync/channels/7/overrides", got.path)
		assert.Equal(t, "docs/a.md", got.body["path"])
		assert.Equal(t, "keep_target", got.body["verdict"])
	})
}

// TestHistsyncPushForceIsNotSilentlyIgnored `+push --force` 不能被静默忽略。
//
// --force 的语义是 sync 的「忽略阻塞项」,push 要覆盖远端已有提交得用 --force-push。
// 静默丢弃会让人以为覆盖了分叉检查。
func TestHistsyncPushForceIsNotSilentlyIgnored(t *testing.T) {
	cmd := histsyncSubCmd(t, "+push")
	require.NoError(t, cmd.Flags().Set("channel-id", "7"))
	require.NoError(t, cmd.Flags().Set("force", "true"))
	// 闸门在 cobra 的 PreRunE 上(命令执行的必经之路)。
	err := cmd.PreRunE(cmd, nil)
	require.Error(t, err, "--force 在 push 上必须明确报错,不能静默丢弃")
	assert.Contains(t, err.Error(), "--force-push")
}

// TestHistsyncRejectsFlagsOfOtherCommands 统一闸门:任何「本命令不认」的参数
// 一旦被显式设置,命令必须报错退出 —— 而不是静默丢弃。
//
// 实测过:CLI 过去把 29 个 flag 撒在每个子命令上,21 个命令共 **578 处静默丢弃**
// (`+plan --force-push`、`+channel-list --verdict merged` 都被接受然后丢掉)。
// 静默丢弃比报错危险:它让人以为操作生效了(而服务端根本没收到那个参数)。
func TestHistsyncRejectsFlagsOfOtherCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		flag string
	}{
		{"+plan", "force-push"},
		{"+plan", "force"},
		{"+apply", "force-push"},
		{"+preview", "force"},
		{"+channel-list", "verdict"},
		{"+channel-list", "url"},
		{"+runs", "run-id"},
		{"+run-detail", "channel-id"},
		{"+override-list", "path"},
		{"+rollback", "channel-id"},
	}
	for _, c := range cases {
		t.Run(c.cmd+" --"+c.flag, func(t *testing.T) {
			cmd := histsyncSubCmd(t, c.cmd)
			require.NoError(t, cmd.Flags().Set(c.flag, "true"))
			err := cmd.PreRunE(cmd, nil)
			require.Error(t, err, "%s 不该接受 --%s", c.cmd, c.flag)
			assert.Contains(t, err.Error(), "--"+c.flag)
		})
	}
}

// TestHistsyncAcceptsItsOwnFlags 反向:本命令**该认**的 flag 不能被误拒。
func TestHistsyncAcceptsItsOwnFlags(t *testing.T) {
	cases := []struct {
		cmd  string
		flag string
	}{
		{"+plan", "channel-id"},
		{"+push", "channel-id"},
		{"+push", "force-push"},
		{"+sync", "force"},
		{"+sync", "force-push"},
		{"+run-detail", "run-id"},
		{"+rollback", "ref"},
		{"+override-set", "verdict"},
		{"+target-create", "url"},
		{"+target-rotate", "expires-at"},
	}
	for _, c := range cases {
		t.Run(c.cmd+" --"+c.flag, func(t *testing.T) {
			cmd := histsyncSubCmd(t, c.cmd)
			require.NoError(t, cmd.Flags().Set(c.flag, "true"))
			require.NoError(t, cmd.PreRunE(cmd, nil), "%s 应当接受 --%s", c.cmd, c.flag)
		})
	}
}

// TestHistsyncCredentialFlagsOnlyOnTargetCommands 凭据 flag 不能挂在每个命令上。
//
// 以前 `+plan --credential SECRET` 会被接受然后丢弃 ——
// secret 照样进了 shell 历史,用户却以为设上了。
func TestHistsyncCredentialFlagsOnlyOnTargetCommands(t *testing.T) {
	for _, name := range []string{"+target-create", "+target-rotate"} {
		cmd := histsyncSubCmd(t, name)
		assert.NotNil(t, cmd.Flags().Lookup("credential"),
			"%s 应当有 --credential", name)
		assert.NotNil(t, cmd.Flags().Lookup("credential-file"),
			"%s 应当有 --credential-file", name)
	}
	for _, name := range []string{"+plan", "+preview", "+push", "+override-set", "+channel-list"} {
		cmd := histsyncSubCmd(t, name)
		assert.Nil(t, cmd.Flags().Lookup("credential"),
			"%s 不该有 --credential(会被静默丢弃)", name)
		assert.Nil(t, cmd.Flags().Lookup("credential-file"),
			"%s 不该有 --credential-file(会被静默丢弃)", name)
	}
}

// TestHistsyncWriteCommandsConfirm 写裁决 / 建通道 / 建目标 / apply 都要有人工确认。
//
// MCP 侧有 dangerGuard,CLI 侧原先没有 → 两条路径口径不一致,
// 而 CLI 恰好是 Skill 文档里让 AI 用的路径。
func TestHistsyncWriteCommandsConfirm(t *testing.T) {
	oldYes := cmdutil.Yes
	cmdutil.Yes = false
	defer func() { cmdutil.Yes = oldYes }()

	for _, tc := range []struct {
		name string
		set  map[string]string
	}{
		{"+override-set", map[string]string{"channel-id": "7", "path": "p", "verdict": "keep_target"}},
		{"+channel-create", map[string]string{"name": "c"}},
		{"+channel-update", map[string]string{"channel-id": "7"}},
		{"+target-create", map[string]string{"channel-id": "7", "name": "t"}},
		{"+apply", map[string]string{"channel-id": "7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := histsyncSubCmd(t, tc.name)
			for k, v := range tc.set {
				require.NoError(t, cmd.Flags().Set(k, v))
			}
			env, err := histsyncWriters[tc.name](client.New("http://127.0.0.1:1", "k"), &config.Config{}, cmd, nil)
			require.NoError(t, err)
			require.NotNil(t, env, "%s 未确认时应当返回拒绝信封", tc.name)
			require.False(t, env.OK, "%s 未确认时不该执行", tc.name)
		})
	}
}

// histsyncWriters 是被测的写操作实现(按子命令名)。
var histsyncWriters = map[string]func(*client.Client, *config.Config, *cobra.Command, []string) (*output.Envelope, error){
	"+override-set":   histsyncOverrideSet,
	"+channel-create": histsyncChannelCreate,
	"+channel-update": histsyncChannelUpdate,
	"+target-create":  histsyncTargetCreate,
	"+apply":          histsyncApply,
}
