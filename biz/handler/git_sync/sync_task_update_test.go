package git_sync

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// taskUpdateEngine 只挂任务更新端点。
func taskUpdateEngine() *hertzserver.Hertz {
	h := hertzserver.Default()
	h.POST("/api/v1/sync/task/update", TaskUpdate)
	return h
}

// postTaskUpdate POST 更新端点,返回 (状态码, 响应 data 原文)。
func postTaskUpdate(t *testing.T, h *hertzserver.Hertz, body string) (int, string) {
	t.Helper()
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/sync/task/update",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body=%s", w.Body.String())
	if env.Data == nil {
		env.Data = []byte("null")
	}
	return w.Code, string(env.Data)
}

// seedTask 建一条带全套开关的任务,用于验证"部分更新不动其它字段"。
// CreateTask 自己生成 key,直接返回建好的任务。
func seedTask(t *testing.T, name string) *corebridge.SyncTask {
	t.Helper()
	keep := true
	task, err := GetSyncService().CreateTask(context.Background(), &corebridge.CreateTaskRequest{
		Name: name, SourceRepoKey: "src/repo", SourceBranch: "main",
		TargetRepoKey: "dst/repo", TargetBranch: "main", SyncMode: "single",
		Cron: "0 3 * * *", GitTags: true, GitForce: true, GitPrune: true,
		GitLFS: true, SyncWiki: true, GitBundle: true, Submodules: true, GitPushPrune: true,
		KeepDivergent: &keep, ForcePushPolicy: "block",
	})
	require.NoError(t, err)
	require.NotNil(t, task)
	return task
}

// TestTaskUpdate_PartialBody_KeepsUntouchedFields 钉住"部分更新只改请求里带的字段"。
//
// 回归背景:UpdateTaskReq 的 bool 字段曾声明为非 optional bool,壳层 handler 用
// &req.Enabled 之类取地址后传给 core,指针恒非 nil ⇒ core 的"nil 即不修改"语义失效。
// 于是任何**部分**更新(例如只带 cron)都会把 enabled、git_tags 等全部布尔开关
// 静默清零 —— 任务被无声禁用。这条用例把该语义钉死:改 cron 不得动其它开关。
func TestTaskUpdate_PartialBody_KeepsUntouchedFields(t *testing.T) {
	before := seedTask(t, "seed-partial-update")
	require.True(t, before.Enabled)
	require.True(t, before.GitTags)

	h := taskUpdateEngine()
	code, _ := postTaskUpdate(t, h, `{"key":"`+before.Key+`","cron":"0 4 * * *"}`)
	require.Equal(t, http.StatusOK, code)

	after, err := GetSyncService().GetTask(context.Background(), before.Key)
	require.NoError(t, err)
	require.Equal(t, "0 4 * * *", after.Cron, "cron 应当被更新")
	require.True(t, after.Enabled, "部分更新不得禁用任务(enabled 未出现在请求里)")
	require.True(t, after.GitTags, "部分更新不得清空 git_tags")
	require.True(t, after.GitForce, "部分更新不得清空 git_force")
	require.True(t, after.GitPrune, "部分更新不得清空 git_prune")
	require.True(t, after.GitLFS, "部分更新不得清空 git_lfs")
	require.True(t, after.SyncWiki, "部分更新不得清空 sync_wiki")
	require.True(t, after.GitBundle, "部分更新不得清空 git_bundle")
	require.True(t, after.Submodules, "部分更新不得清空 submodules")
	require.True(t, after.GitPushPrune, "部分更新不得清空 git_push_prune")
	require.Equal(t, "block", after.ForcePushPolicy, "部分更新不得清空 force_push_policy")
}

// TestTaskUpdate_ExplicitFalse_DisablesTask 反向钉住:显式传 false 仍要生效。
// 部分更新不等于"什么都不改" —— 显式给出的布尔值必须真的写下去。
func TestTaskUpdate_ExplicitFalse_DisablesTask(t *testing.T) {
	before := seedTask(t, "seed-explicit-false")
	require.True(t, before.Enabled)

	h := taskUpdateEngine()
	code, _ := postTaskUpdate(t, h, `{"key":"`+before.Key+`","enabled":false,"git_tags":false}`)
	require.Equal(t, http.StatusOK, code)

	after, err := GetSyncService().GetTask(context.Background(), before.Key)
	require.NoError(t, err)
	require.False(t, after.Enabled, "显式 enabled=false 必须生效")
	require.False(t, after.GitTags, "显式 git_tags=false 必须生效")
}
