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
	require.Contains(t, out, `sync_runs_total{status="success"} 1`)
	require.Contains(t, out, "sync_last_run_timestamp_seconds 1700000000")
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
	assert.True(t, strings.Contains(out, "x_total") )
}
