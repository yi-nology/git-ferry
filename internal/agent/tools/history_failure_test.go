package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// TestGetSyncHealth_ReportsHistoryQueryFailure 历史查询失败必须报出来。
//
// 回归背景(P1,"事实被说反"):以前 `if herr == nil && len(runs) > 0` 的 else 分支
// 同时覆盖了"没有历史"和"查询失败"两种情况 ⇒ 报告写「从未执行」,
// 真相是「查不到」⇒ 运维会去查一个根本不存在的配置问题。
//
// 这条用例是**反向验证**:去掉 failures 的记录,本用例会变红。
func TestGetSyncHealth_ReportsHistoryQueryFailure(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.HistoryErr = errors.New("sync_runs 表不可读")

	reg := NewRegistry(m)
	out, err := reg.ByName("get_sync_health").InvokableRun(context.Background(), `{"limit":10}`)
	require.NoError(t, err)

	var body struct {
		Items []struct {
			Key          string `json:"key"`
			HistoryError string `json:"history_error"`
		} `json:"items"`
		Failures []struct {
			Source  string `json:"source"`
			TaskKey string `json:"task_key"`
			Error   string `json:"error"`
		} `json:"failures"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "输出:%s", out)

	require.Len(t, body.Failures, 1, "历史查询失败必须被记下来")
	assert.Equal(t, "history", body.Failures[0].Source)
	assert.Equal(t, "t1", body.Failures[0].TaskKey)
	assert.Contains(t, body.Failures[0].Error, "sync_runs")

	require.Len(t, body.Items, 1)
	assert.NotEmpty(t, body.Items[0].HistoryError,
		"任务级别也要标出历史查询失败(分数是保守打分,不是真实健康度)")
}

// TestGetSyncHealth_NoHistoryIsNotAFailure 真的没有历史时不该报失败。
func TestGetSyncHealth_NoHistoryIsNotAFailure(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.Runs = nil

	reg := NewRegistry(m)
	out, err := reg.ByName("get_sync_health").InvokableRun(context.Background(), `{"limit":10}`)
	require.NoError(t, err)

	var body struct {
		Items []struct {
			HistoryError string `json:"history_error"`
		} `json:"items"`
		Failures []any `json:"failures"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "输出:%s", out)
	assert.Empty(t, body.Failures, "没有历史是正常状态,不是失败")
	require.Len(t, body.Items, 1)
	assert.Empty(t, body.Items[0].HistoryError)
}

// TestGetOpsTodo_PropagatesHealthFailures 待办队列必须把健康维度内部的
// 查询失败也带上来(否则又被这里悄悄吃掉)。
func TestGetOpsTodo_PropagatesHealthFailures(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.HistoryErr = errors.New("sync_runs 表不可读")

	reg := NewRegistry(m)
	out, err := reg.ByName("get_ops_todo").InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)

	var res todoResult
	require.NoError(t, json.Unmarshal([]byte(out), &res), "输出:%s", out)

	assert.True(t, res.Partial)
	require.NotEmpty(t, res.Failures, "健康维度的查询失败必须传到待办队列")
	found := false
	for _, f := range res.Failures {
		if f.Source == "health/history" {
			found = true
			assert.Contains(t, f.Error, "sync_runs")
		}
	}
	assert.True(t, found, "失败来源应标成 health/history,实际=%+v", res.Failures)
}

// TestDiagnoseRun_QueryFailureIsNotNotFound 历史查不到时不能说"未找到"。
//
// 回归背景(P1,"事实被说反"):以前 findRunView 对历史查询失败是 `continue`
// 直接跳过 ⇒ 搜完报 "run N not found" ⇒ 运维以为记录被删了,
// 真相是"查不到" ⇒ 无法断言它不存在。
//
// 这条用例是**反向验证**:把 historyQueryError 分支去掉,本用例会变红。
func TestDiagnoseRun_QueryFailureIsNotNotFound(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.HistoryErr = errors.New("sync_runs 表不可读")

	reg := NewRegistry(m)
	out, err := reg.ByName("diagnose_run").InvokableRun(context.Background(), `{"run_id":999}`)
	require.NoError(t, err)

	var body struct {
		Found       bool   `json:"found"`
		QueryFailed bool   `json:"query_failed"`
		Message     string `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "输出:%s", out)

	assert.False(t, body.Found)
	assert.True(t, body.QueryFailed, "查询失败必须与「确实不存在」区分开")
	assert.Contains(t, body.Message, "无法断言它不存在")
	assert.NotContains(t, body.Message, "未找到 run 999")
}

// TestDiagnoseRun_ReallyNotFound 真的没有时才是"未找到"。
func TestDiagnoseRun_ReallyNotFound(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.Runs = []*corebridge.SyncRun{{ID: 1, TaskKey: "t1", Status: "success"}}

	reg := NewRegistry(m)
	out, err := reg.ByName("diagnose_run").InvokableRun(context.Background(), `{"run_id":999}`)
	require.NoError(t, err)

	var body struct {
		Found       bool   `json:"found"`
		QueryFailed bool   `json:"query_failed"`
		Message     string `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "输出:%s", out)
	assert.False(t, body.Found)
	assert.False(t, body.QueryFailed, "确实不存在时不该标 query_failed")
	assert.Contains(t, body.Message, "未找到 run 999")
}

// TestDeepAnalyze_ReportsHistoryQueryFailure 深度分析必须报出历史查询失败。
//
// 回归背景(P1,"事实被说反"):以前 `if herr == nil && len(runs) > 0` 的 else 分支
// 同时覆盖"没有历史"与"查询失败" ⇒ 报告写「从未执行」,真相是「查不到」。
//
// 这条用例是**反向验证**:去掉 query_failures 的记录,本用例会变红。
func TestDeepAnalyze_ReportsHistoryQueryFailure(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "t1"}}
	m.HistoryErr = errors.New("sync_runs 表不可读")

	reg := NewRegistry(m)
	out, err := reg.ByName("deep_analyze").InvokableRun(context.Background(), `{"question":"哪里有问题"}`)
	require.NoError(t, err)

	var body struct {
		QueryFailures []struct {
			Source  string `json:"source"`
			TaskKey string `json:"task_key"`
			Error   string `json:"error"`
		} `json:"query_failures"`
		Recent []struct {
			TaskKey      string `json:"task_key"`
			HistoryError string `json:"history_error"`
		} `json:"recent"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "输出:%s", out)

	require.Len(t, body.QueryFailures, 1, "历史查询失败必须被记进 query_failures")
	assert.Equal(t, "history", body.QueryFailures[0].Source)
	assert.Equal(t, "t1", body.QueryFailures[0].TaskKey)
	assert.Contains(t, body.QueryFailures[0].Error, "sync_runs")

	require.Len(t, body.Recent, 1)
	assert.NotEmpty(t, body.Recent[0].HistoryError, "任务级别也要标出历史查询失败")
}
