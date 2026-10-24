package git_sync

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	handler "github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// httpRepo 造一个仓库(工作树或裸仓库),返回路径。
func httpRepo(t *testing.T, dir string, bare bool, files map[string]string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	r, err := git.PlainInit(dir, bare)
	require.NoError(t, err)
	if bare {
		return dir
	}
	wt, err := r.Worktree()
	require.NoError(t, err)
	for p, content := range files {
		full := filepath.Join(dir, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	require.NoError(t, wt.AddWithOptions(&git.AddOptions{All: true}))
	sig := &object.Signature{Name: "http", Email: "http@local", When: time.Unix(1700000000, 0)}
	_, err = wt.Commit("init", &git.CommitOptions{Author: sig, Committer: sig})
	require.NoError(t, err)
	head, err := r.Head()
	require.NoError(t, err)
	require.NoError(t, r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("main"), head.Hash())))
	require.NoError(t, r.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName("main"))))
	return dir
}

// httpSeedBare 把工作仓的 main 灌进裸仓库(当推送目标的本地替身)。
func httpSeedBare(t *testing.T, bareDir, srcDir string) {
	t.Helper()
	r, err := git.PlainOpen(bareDir)
	require.NoError(t, err)
	_, err = r.CreateRemote(&config.RemoteConfig{Name: "seed", URLs: []string{srcDir}})
	require.NoError(t, err)
	require.NoError(t, r.Fetch(&git.FetchOptions{
		RemoteName: "seed",
		RefSpecs:   []config.RefSpec{"refs/heads/main:refs/heads/main"},
	}))
	ref, err := r.Reference(plumbing.NewBranchReferenceName("main"), true)
	require.NoError(t, err)
	require.NoError(t, r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("main"), ref.Hash())))
}

// httpPost 发一个 JSON 请求,返回状态码与响应体。
func httpPost(t *testing.T, h *server.Hertz, path string, body map[string]any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := ut.PerformRequest(h.Engine, http.MethodPost, path, &ut.Body{
		Body: bytes.NewReader(raw), Len: len(raw),
	}, ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-API-Key", Value: testAPIKey})
	return w.Code, w.Body.String()
}

// TestHistsyncHTTPEndToEnd HTTP 层的端到端:建通道 → 配推送目标 → 预检 →
// 移植 → 推送 → 校验审计链。
//
// 这是**壳层**的门禁:handler 的参数绑定、鉴权中间件、RBAC、审计落库,
// 以及 core 的整条流水线,要在**真实 HTTP 链路**上串通。
// (core 层另有一条端到端;两条路径的参数形状不同,不能互相替代。)
func TestHistsyncHTTPEndToEnd(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	cfg := &corebridge.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "file:" + filepath.Join(t.TempDir(), "e2e.db")
	cfg.Git.TempDir = filepath.Join(t.TempDir(), "git")
	svc, err := corebridge.NewService(cfg)
	require.NoError(t, err)
	handler.SetSyncServiceGetter(func() *corebridge.Service { return svc })
	handler.SetAPIKey(testAPIKey)

	h := server.Default()
	Register(h)

	root := t.TempDir()
	srcDir := httpRepo(t, filepath.Join(root, "src"), false, map[string]string{
		"go.mod": "module git.enjoye.top/enjoydream/app\n",
		"a.go":   "package app\n\n// 新增\nfunc A() {}\n",
	})
	tgtDir := httpRepo(t, filepath.Join(root, "tgt"), false, map[string]string{
		"go.mod": "module github.com/yi-nology/app\n",
		"b.go":   "package app\n",
	})
	backupDir := httpRepo(t, filepath.Join(root, "backup.git"), true, nil)
	publishDir := httpRepo(t, filepath.Join(root, "publish.git"), true, nil)
	httpSeedBare(t, backupDir, tgtDir)
	httpSeedBare(t, publishDir, tgtDir)

	// 建通道。
	code, body := httpPost(t, h, "/api/v1/histsync/channels", map[string]any{
		"name": "http-e2e", "source_dir": srcDir, "target_dir": tgtDir,
		"module_source": "git.enjoye.top/enjoydream/app",
		"module_target": "github.com/yi-nology/app",
		"source_branch": "main", "target_branch": "main", "enabled": true,
	})
	require.Equal(t, http.StatusOK, code, "建通道失败: %s", body)
	var created struct {
		ID uint `json:"id"`
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	created = env.Data
	require.NotZero(t, created.ID, "建通道响应里应当有 id: %s", body)
	chPath := "/api/v1/histsync/channels/" + strconv.FormatUint(uint64(created.ID), 10)

	// 配推送目标(先留底后发布)。
	for _, tgt := range []struct{ name, url string }{
		{"backup", backupDir}, {"publish", publishDir},
	} {
		code, body = httpPost(t, h, chPath+"/targets", map[string]any{
			"name": tgt.name, "repo_url": tgt.url, "cred_type": "none",
		})
		require.Equal(t, http.StatusOK, code, "建推送目标失败: %s", body)
	}

	// 预检(不推送)→ 移植 → 推送。
	for _, step := range []string{"/preview", "/apply", "/push"} {
		code, body = httpPost(t, h, chPath+step, nil)
		require.Equal(t, http.StatusOK, code, "%s 失败: %s", step, body)
	}

	// 发布目标真的前进了。
	pub, err := git.PlainOpen(publishDir)
	require.NoError(t, err)
	head, err := pub.Reference(plumbing.NewBranchReferenceName("main"), true)
	require.NoError(t, err)
	commit, err := pub.CommitObject(head.Hash())
	require.NoError(t, err)
	tree, err := commit.Tree()
	require.NoError(t, err)
	_, err = tree.FindEntry("a.go")
	require.NoError(t, err, "发布目标里应有移植过去的文件")

	// 审计链完整。
	w := ut.PerformRequest(h.Engine, http.MethodGet, chPath+"/audit", nil,
		ut.Header{Key: "X-API-Key", Value: testAPIKey})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":true`, "审计链应完整: %s", w.Body.String())
}
