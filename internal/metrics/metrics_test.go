package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_CounterAndLabeled(t *testing.T) {
	r := New(func() time.Time { return time.Unix(1700000000, 0) })
	r.IncCounter("process_up", "up")
	r.AddLabeled("http_requests_total", "http", map[string]string{"method": "GET", "path": "/ping", "status": "200"}, 1)
	r.AddLabeled("http_requests_total", "http", map[string]string{"method": "GET", "path": "/ping", "status": "200"}, 2)
	r.ObserveSyncRun("success")

	out := r.WritePrometheus()
	require.Contains(t, out, "process_up 1")
	require.Contains(t, out, `http_requests_total{method="GET",path="/ping",status="200"} 3`)
	// 无 task 的打点落在 task="" 序列(Prometheus 摄入时空标签等价于不存在)
	require.Contains(t, out, `sync_runs_total{status="success",task=""} 1`)
	// client_golang 文本格式按最短浮点表示输出 gauge
	require.Contains(t, out, "sync_last_run_timestamp_seconds 1.7e+09")
}

func TestRegistry_SyncRunWithTask_SharesOneSeriesFamily(t *testing.T) {
	r := New(nil)
	r.ObserveSyncRunWithTask("t1", "failed")
	r.ObserveSyncRun("failed")
	out := r.WritePrometheus()
	require.Contains(t, out, `sync_runs_total{status="failed",task="t1"} 1`)
	require.Contains(t, out, `sync_runs_total{status="failed",task=""} 1`)
}

func TestLowCardinalityPath(t *testing.T) {
	assert.Equal(t, "/api/v1/repo/:id", LowCardinalityPath("/api/v1/repo/42"))
	assert.Equal(t, "/health", LowCardinalityPath("/health"))
	assert.Equal(t, "/a/:id/b", LowCardinalityPath("/a/123/b"))
	// 非纯数字保留
	assert.Equal(t, "/api/v1/ai/chat", LowCardinalityPath("/api/v1/ai/chat"))
}

func TestWritePrometheus_LabelEscaping(t *testing.T) {
	r := New(nil)
	r.AddLabeled("x_total", "h", map[string]string{"status": "success"}, 1)
	out := r.WritePrometheus()
	assert.True(t, strings.Contains(out, "x_total"))
}
