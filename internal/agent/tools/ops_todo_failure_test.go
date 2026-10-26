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

// todoResult 是 get_ops_todo 的输出形状(只取断言需要的字段)。
type todoResult struct {
	Items     []map[string]any `json:"items"`
	Total     int              `json:"total"`
	Returned  int              `json:"returned"`
	Partial   bool             `json:"partial"`
	Truncated bool             `json:"truncated"`
	Warning   string           `json:"warning"`
	Message   string           `json:"message"`
	Failures  []struct {
		Source string `json:"source"`
		Error  string `json:"error"`
	} `json:"failures"`
}

func runOpsTodo(t *testing.T, m *toolstest.Mock) todoResult {
	t.Helper()
	reg := NewRegistry(m)
	out, err := reg.ByName("get_ops_todo").InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	var res todoResult
	require.NoError(t, json.Unmarshal([]byte(out), &res), "输出:%s", out)
	return res
}

// TestGetOpsTodo_ReportsSourceFailures 子数据源失败**必须显式暴露**。
//
// 回归背景(P0,"报告与事实相反"):健康 / 孤儿两路走子工具,子工具失败时
// 返回的是 `{"error": "..."}` 的 JSON 而 **Go error 恒为 nil**
// ⇒ 以前 `json.Unmarshal` 不报错、Items 为空
// ⇒ 子系统一挂,这一整类待办**无声消失**,调用方看到的是"没有待办";
// RPO 一路连 Go error 都被直接丢掉。
//
// 这条用例是**反向验证**:去掉 failures 的收集,本用例会变红。
func TestGetOpsTodo_ReportsSourceFailures(t *testing.T) {
	m := toolstest.NewMock()
	m.TaskErr = errors.New("db down")
	m.RepoErr = errors.New("repo db down")
	m.RPOErr = errors.New("backup volume unreachable")

	res := runOpsTodo(t, m)

	assert.True(t, res.Partial, "有数据源失败时必须标记 partial")
	require.Len(t, res.Failures, 3, "三路数据源失败都要被记下来:实际=%+v", res.Failures)

	sources := map[string]string{}
	for _, f := range res.Failures {
		sources[f.Source] = f.Error
	}
	assert.Contains(t, sources["health"], "db down", "健康数据源的失败原因要能看到")
	assert.Contains(t, sources["inventory"], "repo db down", "孤儿数据源的失败原因要能看到")
	assert.Contains(t, sources["rpo"], "unreachable", "RPO 数据源的失败原因要能看到")

	assert.NotEmpty(t, res.Warning, "必须有一句明说的警告,不能让人把不完整当完整")
	assert.Contains(t, res.Warning, "不要据此断言")
}

// TestGetOpsTodo_AllSourcesFailed_NotEmptyList 全挂时不能看起来像"没有待办"。
func TestGetOpsTodo_AllSourcesFailed_NotEmptyList(t *testing.T) {
	m := toolstest.NewMock()
	m.TaskErr = errors.New("db down")
	m.RepoErr = errors.New("repo db down")
	m.RPOErr = errors.New("backup down")

	res := runOpsTodo(t, m)

	assert.True(t, res.Partial)
	assert.Empty(t, res.Items)
	assert.NotEmpty(t, res.Message,
		"所有数据源都失败时必须明确说「无法给出待办清单」,而不是给一个空列表")
}

// TestGetOpsTodo_ReportsTruncation 截断必须说清:total 是截断前的真实条数。
//
// 回归背景(P1):以前 total 报的是**截断后**的长度
// ⇒ 模型看到"一共 20 条"却不知道还有更多。
func TestGetOpsTodo_ReportsTruncation(t *testing.T) {
	m := toolstest.NewMock()
	// 造 30 个健康分很低的任务 ⇒ 待办超过默认上限 20
	for i := 0; i < 30; i++ {
		m.Tasks = append(m.Tasks, &corebridge.SyncTask{
			Key: "t" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Name: "bad",
		})
	}
	m.Runs = []*corebridge.SyncRun{{ID: 1, TaskKey: "t", Status: "failed"}}

	reg := NewRegistry(m)
	out, err := reg.ByName("get_ops_todo").InvokableRun(context.Background(), `{"max_items":5}`)
	require.NoError(t, err)
	var res todoResult
	require.NoError(t, json.Unmarshal([]byte(out), &res), "输出:%s", out)

	assert.True(t, res.Truncated, "超出 max_items 必须标记 truncated")
	assert.Equal(t, 5, res.Returned, "实际返回条数")
	assert.Greater(t, res.Total, res.Returned,
		"total 必须是截断前的真实条数(否则模型以为只有 5 条)")
}

// TestGetOpsTodo_NoFailure_NotPartial 一切正常时不该挂 partial 标记。
func TestGetOpsTodo_NoFailure_NotPartial(t *testing.T) {
	m := toolstest.NewMock()
	m.Tasks = []*corebridge.SyncTask{{Key: "t1", Name: "ok"}}
	m.Runs = []*corebridge.SyncRun{{ID: 1, TaskKey: "t1", Status: "success"}}

	res := runOpsTodo(t, m)

	assert.False(t, res.Partial, "没有数据源失败时不该报 partial")
	assert.Empty(t, res.Failures)
}

// TestListHistoryRuns_PaginatesAndReportsTruncation 执行记录列表必须真分页,
// 且截断要说出口。
//
// 回归背景(P1,静默截断):以前硬编码 50 条、且入参只有 channel_id
// ⇒ 模型想看更多也拿不到,而输出里的 total 却是**真实总数**
// ⇒ 两个数字对不上,模型无从判断自己看到的是不是全部。
//
// 这条用例是**反向验证**:把分页改回硬编码 50,本用例会变红。
func TestListHistoryRuns_PaginatesAndReportsTruncation(t *testing.T) {
	m := toolstest.NewMock()
	for i := 0; i < 120; i++ {
		m.HistoryRuns = append(m.HistoryRuns, &corebridge.HistoryRunDTO{ID: uint(i + 1)})
	}
	m.HistoryRunsTotal = 120

	reg := NewRegistry(m)
	out, err := reg.ByName("list_histsync_runs").InvokableRun(context.Background(),
		`{"channel_id":1,"page":2,"limit":20}`)
	require.NoError(t, err)

	var res struct {
		Total     int                        `json:"total"`
		Page      int                        `json:"page"`
		Limit     int                        `json:"limit"`
		Returned  int                        `json:"returned"`
		Truncated bool                       `json:"truncated"`
		Notice    string                     `json:"notice"`
		Runs      []corebridge.HistoryRunDTO `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res), "输出:%s", out)

	assert.Equal(t, 20, m.HistoryRunsLimit, "limit 必须传给服务端(不能硬编码 50)")
	assert.Equal(t, 20, m.HistoryRunsOffset, "第 2 页的 offset 应为 (page-1)*limit")
	assert.Equal(t, 20, res.Returned)
	assert.Equal(t, 120, res.Total)
	assert.True(t, res.Truncated, "还有更多记录时必须标记 truncated")
	assert.NotEmpty(t, res.Notice, "截断要有明说的提示")
	assert.Len(t, res.Runs, 20)
}

// TestListHistoryRuns_DefaultLimit 默认走 20 条(不是硬编码 50)。
func TestListHistoryRuns_DefaultLimit(t *testing.T) {
	m := toolstest.NewMock()
	m.HistoryRuns = []*corebridge.HistoryRunDTO{{ID: 1}}
	m.HistoryRunsTotal = 1

	reg := NewRegistry(m)
	_, err := reg.ByName("list_histsync_runs").InvokableRun(context.Background(), `{"channel_id":1}`)
	require.NoError(t, err)

	assert.Equal(t, 20, m.HistoryRunsLimit, "默认 limit 应为 20")
	assert.Equal(t, 0, m.HistoryRunsOffset)
}

// TestParseToolPayload_识别错误信封 parseToolPayload 必须把 {"error":...}
// 负载变成 Go error —— 这是"子工具失败被静默吞掉"的根因。
func TestParseToolPayload_识别错误信封(t *testing.T) {
	var out struct {
		Items []string `json:"items"`
	}
	err := parseToolPayload(`{"error":"查询任务失败: db down"}`, &out)
	require.Error(t, err, "错误信封必须被识别成错误")
	assert.Contains(t, err.Error(), "db down")

	// 正常负载
	require.NoError(t, parseToolPayload(`{"items":["a"]}`, &out))
	assert.Equal(t, []string{"a"}, out.Items)

	// 非法 JSON
	require.Error(t, parseToolPayload(`not json`, &out))
}
