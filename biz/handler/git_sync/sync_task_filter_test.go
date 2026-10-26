package git_sync

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// probeStatus 一个正常流程里**不会出现**的状态值:
// 用它做过滤探针,既不会和别的用例的数据撞上,又不会被当成真实状态。
const probeStatus = "filter-probe-status"

func syncTaskEngine() *hertzserver.Hertz {
	h := hertzserver.Default()
	h.GET("/api/v1/sync/tasks", TaskList)
	h.GET("/api/v1/sync/history", TaskHistory)
	return h
}

// TestTaskList_StatusFilterHonored `?status=` 必须是**真的过滤条件**。
//
// 回归背景(P1,契约漂移):CLI 的 `task +list --status` 一直在发 status,
// 但服务端 ListTasksReq 里没有这个字段 ⇒ hertz 静默忽略未知 query 参数
// ⇒ 用户拿到全量却以为筛过了(服务端返回的 total 也是全量)。
//
// 这条用例是**反向验证**:把 handler 改回 ListTasks(不过滤),本用例会变红。
func TestTaskList_StatusFilterHonored(t *testing.T) {
	svc := GetSyncService()
	ctx := context.Background()

	hit, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-hit", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)
	miss, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-miss", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)

	// 只给 hit 打上探针状态。
	require.NoError(t, setProbeLastStatus(svc, hit, probeStatus))

	h := syncTaskEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/sync/tasks?status="+probeStatus, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Tasks []struct {
				Key        string `json:"key"`
				LastStatus string `json:"last_status"`
			} `json:"tasks"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, []int{0, 200}, resp.Code, "响应体:%s", w.Body.String())

	assert.Equal(t, int64(1), resp.Data.Total, "过滤后的 total 必须是命中数,不是全量")
	require.Len(t, resp.Data.Tasks, 1)
	assert.Equal(t, hit.Key, resp.Data.Tasks[0].Key, "只能返回命中状态的任务")
	assert.NotEqual(t, miss.Key, resp.Data.Tasks[0].Key)
}

// TestTaskList_NoStatusFilter_ReturnsAll 不带 status 时必须返回全量(不过滤)。
func TestTaskList_NoStatusFilter_ReturnsAll(t *testing.T) {
	svc := GetSyncService()
	ctx := context.Background()
	task, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-plain", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)
	require.NoError(t, setProbeLastStatus(svc, task, probeStatus+"-plain"))

	h := syncTaskEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/sync/tasks", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Greater(t, resp.Data.Total, int64(1),
		"不带 status 时必须返回全量(不能只剩一条)")
}

// TestTaskHistory_RunIDFilterHonored `?run_id=` 必须精确返回那一条。
//
// 回归背景(P1):CLI 的 `history +detail --run-id` 之前只能靠"翻页找那条"——
// 服务端没有按 run_id 过滤的能力 ⇒ 翻不到就静默返回空列表,
// 用户以为"这条记录不存在"。
//
// 这条用例是**反向验证**:把 handler 改回 ListHistory(按任务翻页),本用例会变红。
func TestTaskHistory_RunIDFilterHonored(t *testing.T) {
	svc := GetSyncService()
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-history", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err)
	other, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err)

	h := syncTaskEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet,
		"/api/v1/sync/history?task_key="+task.Key+"&run_id="+itoa(run.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Runs []struct {
				ID uint `json:"id"`
			} `json:"runs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, []int{0, 200}, resp.Code, "响应体:%s", w.Body.String())

	require.Len(t, resp.Data.Runs, 1, "run_id 过滤必须只返回那一条")
	assert.Equal(t, run.ID, resp.Data.Runs[0].ID)
	assert.NotEqual(t, other.ID, resp.Data.Runs[0].ID)
}

// TestTaskHistory_RunIDNotFound 不存在的 run_id 必须报 404,不能静默返回空。
func TestTaskHistory_RunIDNotFound(t *testing.T) {
	svc := GetSyncService()
	ctx := context.Background()
	task, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-missing", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)

	h := syncTaskEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet,
		"/api/v1/sync/history?task_key="+task.Key+"&run_id=999999999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"查不到的 run_id 必须报 404(以前是静默空列表)")
}

// TestTaskHistory_RunIDWithoutTaskKey 只给 run_id 也必须能查(ID 全局唯一)。
func TestTaskHistory_RunIDWithoutTaskKey(t *testing.T) {
	svc := GetSyncService()
	ctx := context.Background()
	task, err := svc.CreateTask(ctx, &corebridge.CreateTaskRequest{
		Name: "probe-byid", SourceRepoKey: "probe/src", TargetRepoKey: "probe/dst",
	})
	require.NoError(t, err)
	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err)

	h := syncTaskEngine()
	w := ut.PerformRequest(h.Engine, http.MethodGet,
		"/api/v1/sync/history?run_id="+itoa(run.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Runs []struct {
				ID uint `json:"id"`
			} `json:"runs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Runs, 1)
	assert.Equal(t, run.ID, resp.Data.Runs[0].ID)
}

// setProbeLastStatus 给任务打上探针状态(last_status)。
func setProbeLastStatus(svc *corebridge.Service, task *corebridge.SyncTask, status string) error {
	run, err := svc.CreateRun(task, "manual", nil)
	if err != nil {
		return err
	}
	now := time.Now()
	run.Status = status
	run.EndTime = &now
	return svc.UpdateTaskLastRun(task, run)
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
