// Package metrics 提供零依赖的 Prometheus 文本格式指标。
// 只覆盖壳层可观测刚需:HTTP 请求、同步运行结果、最近一次运行时间。
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry 进程内指标注册表(线程安全)。
type Registry struct {
	mu       sync.Mutex
	counters map[string]*int64
	gauges   map[string]*atomic.Int64
	labeled  map[string]*int64
	help     map[string]string
	now      func() time.Time
}

// New 创建注册表。now 可注入(测试用)。
func New(now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{
		counters: map[string]*int64{},
		gauges:   map[string]*atomic.Int64{},
		labeled:  map[string]*int64{},
		help:     map[string]string{},
		now:      now,
	}
}

var defaultRegistry = New(nil)

// Default 返回进程默认注册表。
func Default() *Registry { return defaultRegistry }

// IncCounter 无标签计数器 +1。
func (r *Registry) IncCounter(name, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.counters[name]
	if !ok {
		var v int64
		c = &v
		r.counters[name] = c
		r.help[name] = help
	}
	atomic.AddInt64(c, 1)
}

// AddLabeled 有标签计数器 +delta。labels 形如 {"status":"success"}。
func (r *Registry) AddLabeled(name, help string, labels map[string]string, delta int64) {
	key := labeledKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.labeled[key]
	if !ok {
		var v int64
		c = &v
		r.labeled[key] = c
		if help != "" {
			r.help[name] = help
		}
	}
	atomic.AddInt64(c, delta)
}

// SetGauge 设置无标签 gauge。
func (r *Registry) SetGauge(name, help string, v int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.gauges[name]
	if !ok {
		g = &atomic.Int64{}
		r.gauges[name] = g
		r.help[name] = help
	}
	g.Store(v)
}

// ObserveSyncRun 记录一次同步运行结果并刷新最近运行时间戳。
func (r *Registry) ObserveSyncRun(status string) {
	r.AddLabeled("sync_runs_total", "Sync runs by status", map[string]string{"status": status}, 1)
	r.SetGauge("sync_last_run_timestamp_seconds",
		"Unix timestamp of last observed sync run", r.now().Unix())
}

// ObserveSyncRunWithTask 按任务+状态记录（任务数有界，不会打爆基数）。
func (r *Registry) ObserveSyncRunWithTask(taskKey, status string) {
	if taskKey == "" {
		r.ObserveSyncRun(status)
		return
	}
	r.AddLabeled("sync_runs_total", "Sync runs by status",
		map[string]string{"status": status, "task": taskKey}, 1)
	r.SetGauge("sync_last_run_timestamp_seconds",
		"Unix timestamp of last observed sync run", r.now().Unix())
}

func labeledKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return name + "|" + strings.Join(parts, "|")
}

type sample struct {
	key string
	val int64
}

// WritePrometheus 渲染 Prometheus 文本格式。
func (r *Registry) WritePrometheus() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder

	names := make([]string, 0, len(r.help))
	for n := range r.help {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		if h := r.help[name]; h != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", name, h)
		}

		var labeled []sample
		for k, v := range r.labeled {
			if k == name || strings.HasPrefix(k, name+"|") {
				labeled = append(labeled, sample{k, atomic.LoadInt64(v)})
			}
		}

		_, hasCounter := r.counters[name]
		_, hasGauge := r.gauges[name]

		if hasCounter || len(labeled) > 0 && !hasGauge {
			if !hasGauge {
				fmt.Fprintf(&b, "# TYPE %s counter\n", name)
			}
		}
		if hasGauge {
			fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
		}

		if hasCounter {
			fmt.Fprintf(&b, "%s %d\n", name, atomic.LoadInt64(r.counters[name]))
		}
		if hasGauge {
			fmt.Fprintf(&b, "%s %d\n", name, r.gauges[name].Load())
		}
		if len(labeled) > 0 {
			sort.Slice(labeled, func(i, j int) bool { return labeled[i].key < labeled[j].key })
			for _, s := range labeled {
				fmt.Fprintf(&b, "%s%s %d\n", name, formatLabelSuffix(s.key, name), s.val)
			}
		}
	}
	return b.String()
}

func formatLabelSuffix(key, name string) string {
	if key == name {
		return ""
	}
	rest := strings.TrimPrefix(key, name+"|")
	pairs := strings.Split(rest, "|")
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		out = append(out, fmt.Sprintf("%s=%q", kv[0], kv[1]))
	}
	if len(out) == 0 {
		return ""
	}
	return "{" + strings.Join(out, ",") + "}"
}

// HTTPHandler 输出 /metrics。
func HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(defaultRegistry.WritePrometheus()))
	}
}

// ObserveHTTP 记录一次 HTTP 请求到默认注册表。
func ObserveHTTP(method, path string, status int) {
	defaultRegistry.AddLabeled("http_requests_total", "Total HTTP requests",
		map[string]string{"method": method, "path": path, "status": fmt.Sprint(status)}, 1)
}

// ObserveSyncRun 记录到默认注册表。
func ObserveSyncRun(status string) {
	defaultRegistry.ObserveSyncRun(status)
}

// ObserveSyncRunWithTask 记录到默认注册表（带 task 标签）。
func ObserveSyncRunWithTask(taskKey, status string) {
	defaultRegistry.ObserveSyncRunWithTask(taskKey, status)
}

// LowCardinalityPath 把路径中的纯数字段替换为 :id,避免 label 爆炸。
func LowCardinalityPath(p string) string {
	out := make([]byte, 0, len(p))
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			seg := p[start:i]
			if isAllDigits(seg) {
				out = append(out, ":id"...)
			} else {
				out = append(out, seg...)
			}
			if i < len(p) {
				out = append(out, '/')
			}
			start = i + 1
		}
	}
	return string(out)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	_, err := strconv.Atoi(s)
	return err == nil
}
