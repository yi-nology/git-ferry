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
)

// repoSubCmd 取出仓库命令组里的某个子命令。
func repoSubCmd(t *testing.T, name string) *cobra.Command {
	t.Helper()
	root := newRepoCmd()
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("找不到子命令 %s", name)
	return nil
}

// TestRepoCreate_BodyAndQueryMatchServer 钉住 `repo +create` 的请求形状。
//
// 回归背景(P0,命令 100% 失败):CLI 发的是 `platform` / `clone_url` / `key`,
// 而服务端的 CreateRepoReq 只有 **name + remote_url**(平台走 query 的
// **platform_id**)⇒ 三个字段全被静默丢弃 ⇒ RemoteUrl 为空 ⇒ 400
// ⇒ **这个命令从来没能成功过**。
//
// 这条用例是**反向验证**:把 body 改回 clone_url(或不再解析 platform_id),
// 本用例会变红。
func TestRepoCreate_BodyAndQueryMatchServer(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/repo/create" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/platforms":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"platforms":[{"id":7,"key":"github","name":"GitHub"}],"total":1}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"repo":{"key":"o/r"}}}`))
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-key")
	oldYes := cmdutil.Yes
	cmdutil.Yes = true
	defer func() { cmdutil.Yes = oldYes }()

	cmd := repoSubCmd(t, "+create")
	require.NoError(t, cmd.Flags().Set("name", "demo"))
	require.NoError(t, cmd.Flags().Set("platform", "github"))
	require.NoError(t, cmd.Flags().Set("clone-url", "https://github.com/o/demo.git"))

	_, err := repoCreate(c, &config.Config{}, cmd, nil)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/repo/create", gotPath)
	// 请求体字段名必须与服务端模型一致
	assert.Equal(t, "https://github.com/o/demo.git", gotBody["remote_url"],
		"请求体必须是 remote_url(服务端 CreateRepoReq 没有 clone_url)")
	assert.NotContains(t, gotBody, "clone_url", "clone_url 不在服务端模型里,发了会被丢弃")
	assert.NotContains(t, gotBody, "platform", "平台走 query 的 platform_id,不在请求体里")
	assert.Equal(t, "demo", gotBody["name"])
	// 平台 key 必须被解析成 platform_id
	assert.Equal(t, []string{"7"}, gotQuery["platform_id"],
		"--platform 给的是平台 key,必须解析成 platform_id 传给服务端")
}
