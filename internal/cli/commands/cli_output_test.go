package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// captureStdout 捕获 fn 写到 os.Stdout 的内容。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	require.NoError(t, w.Close())
	os.Stdout = old
	return <-done
}

// opsSubCmd 取出运维命令组里的某个子命令。
func opsSubCmd(t *testing.T, name string) *cobra.Command {
	t.Helper()
	root := newOpsCmd()
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("找不到子命令 %s", name)
	return nil
}

// TestOpsAuditReport_CSVStdoutIsPureCSV `--csv` 导出时 stdout **只能有 CSV**。
//
// 回归背景(P1,导出文件是坏的):服务端 CSV 走 render.CSV,响应体是裸 CSV 文本;
// CLI 之前先打印 CSV,再让框架把 Envelope JSON 也打到 stdout
// ⇒ `gitferry ops +audit-report --csv > out.csv` 得到的文件尾部多一段 JSON
// ⇒ 任何 CSV 解析器都读不了这个文件。
//
// 这条用例是**反向验证**:让命令返回一个 Success Envelope,本用例会变红。
func TestOpsAuditReport_CSVStdoutIsPureCSV(t *testing.T) {
	const csvBody = "id,action,resource_type\n7,create,task\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "csv", r.URL.Query().Get("format"), "--csv 必须让服务端走 CSV 分支")
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csvBody))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-key")
	cmd := opsSubCmd(t, "+audit-report")
	require.NoError(t, cmd.Flags().Set("csv", "true"))

	var env any
	out := captureStdout(t, func() {
		e, err := opsAuditReport(c, &config.Config{}, cmd, nil)
		require.NoError(t, err)
		env = e
	})

	assert.Equal(t, csvBody, out, "stdout 必须只有 CSV,不能混入 Envelope JSON")
	assert.NotContains(t, out, "\"ok\"", "Envelope JSON 不能污染 CSV 输出")
	assert.Nil(t, env, "CSV 已经自己输出了,命令不能再返回 Envelope(否则框架会再打一遍)")
}

// TestOpsAuditReport_JsonStillEnvelope 非 CSV 模式仍然返回 Envelope。
func TestOpsAuditReport_JsonStillEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"items":[],"total":0}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-key")
	cmd := opsSubCmd(t, "+audit-report")

	env, err := opsAuditReport(c, &config.Config{}, cmd, nil)
	require.NoError(t, err)
	require.NotNil(t, env)
	assert.True(t, env.OK)
}

// TestAuthStatus_ServerErrorReportsFailure 服务端 4xx/5xx 必须报失败。
//
// 回归背景(P0,"报告与事实相反"):auth status 以前无论服务端返回什么
// 都 output.Success(...)⇒ 信封里永远 ok=true ⇒
// 脚本按 ok 判断就会把"密钥无效"当成"一切正常"。
//
// 这条用例是**反向验证**:把失败分支改回 output.Success,本用例会变红。
func TestAuthStatus_ServerErrorReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"invalid api key"}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "bad-key")
	info, env := authStatus(c, &config.Config{BaseURL: srv.URL})

	require.NotNil(t, env)
	assert.False(t, env.OK, "401 必须报失败,不能 ok=true")
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "invalid api key")
	assert.Equal(t, false, info["reachable"], "服务不可达/被拒时 reachable 必须是 false")
}

// TestAuthStatus_SuccessReportsOK 正常时仍然报 ok。
func TestAuthStatus_SuccessReportsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"version":"v1"}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "good-key")
	info, env := authStatus(c, &config.Config{BaseURL: srv.URL})

	require.NotNil(t, env)
	assert.True(t, env.OK)
	assert.Equal(t, true, info["reachable"])
}

// TestAuthStatus_UnreachableServerReportsFailure 服务不可达也必须报失败。
func TestAuthStatus_UnreachableServerReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close() // 立刻关掉 ⇒ 连接必然失败

	c := client.New(addr, "k")
	info, env := authStatus(c, &config.Config{BaseURL: addr})

	require.NotNil(t, env)
	assert.False(t, env.OK, "服务不可达必须报失败")
	assert.Equal(t, false, info["reachable"])
}

// TestEnvelopeFromServer_ErrorNotOK 确认信封层本身对 4xx 的判定:
// 这正是 auth status 之前"永远 ok=true"的根因。
func TestEnvelopeFromServer_ErrorNotOK(t *testing.T) {
	env := output.FromServer(401, []byte(`{"code":401,"message":"nope"}`))
	assert.False(t, env.OK, "4xx 必须映射成失败信封")
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "nope")
}
