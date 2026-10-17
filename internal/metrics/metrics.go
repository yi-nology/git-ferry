// Package metrics 提供壳层可观测指标:基于 prometheus/client_golang,默认注册表复用
// oekit metrics.Registry(自带 Go runtime / process 采集器与 app_build_info),
// /metrics 端点由 oekit hertz/metrics 暴露。只覆盖壳层刚需:HTTP 请求、同步运行结果、
// 最近一次运行时间、平台 API 限流回退。
package metrics

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	oemetrics "github.com/oekit/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// serviceName 进入 oekit app_build_info{service} 标签。
const serviceName = "git-ferry"

// Registry 进程内指标注册表(线程安全)。
type Registry struct {
	reg      *prometheus.Registry
	mu       sync.Mutex
	counters map[string]prometheus.Counter
	gauges   map[string]prometheus.Gauge
	vecs     map[string]*labeledVec
	now      func() time.Time
}

// labeledVec 标签名在首次注册时固定:client_golang 要求同名指标的标签名一致。
type labeledVec struct {
	keys []string
	vec  *prometheus.CounterVec
}

// New 创建独立注册表。now 可注入(测试用)。
func New(now func() time.Time) *Registry {
	return newRegistry(prometheus.NewRegistry(), now)
}

func newRegistry(reg *prometheus.Registry, now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	r := &Registry{
		reg:      reg,
		counters: map[string]prometheus.Counter{},
		gauges:   map[string]prometheus.Gauge{},
		vecs:     map[string]*labeledVec{},
		now:      now,
	}
	// sync_runs_total 有/无 task 两种打点形态,统一为 {status,task}:无 task 时值为空串,
	// Prometheus 摄入时空标签等价于不存在,查询语义与此前分开两条序列一致。
	r.registerVec("sync_runs_total", "Sync runs by status", []string{"status", "task"})
	return r
}

var defaultRegistry = func() *Registry {
	oemetrics.Init(serviceName)
	return newRegistry(oemetrics.Registry, nil)
}()

// Default 返回进程默认注册表(底层为 oekit metrics.Registry)。
func Default() *Registry { return defaultRegistry }

// registerVec 调用方须持有 r.mu。
func (r *Registry) registerVec(name, help string, keys []string) *labeledVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: helpOrName(help, name)}, keys)
	r.reg.MustRegister(v)
	lv := &labeledVec{keys: keys, vec: v}
	r.vecs[name] = lv
	return lv
}

// helpOrName client_golang 拒绝空 Help,沿用旧 API 允许不传 help 的习惯时退化为指标名。
func helpOrName(help, name string) string {
	if help == "" {
		return name
	}
	return help
}

// IncCounter 无标签计数器 +1。
func (r *Registry) IncCounter(name, help string) {
	r.mu.Lock()
	c, ok := r.counters[name]
	if !ok {
		c = prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: helpOrName(help, name)})
		r.reg.MustRegister(c)
		r.counters[name] = c
	}
	r.mu.Unlock()
	c.Inc()
}

// AddLabeled 有标签计数器 +delta(delta<=0 忽略)。labels 形如 {"status":"success"};
// 标签名以该指标首次出现时为准,之后缺失的标签取空串、多余的忽略。
func (r *Registry) AddLabeled(name, help string, labels map[string]string, delta int64) {
	if delta <= 0 {
		return
	}
	r.mu.Lock()
	lv, ok := r.vecs[name]
	if !ok {
		lv = r.registerVec(name, help, sortedKeys(labels))
	}
	r.mu.Unlock()
	vals := make([]string, len(lv.keys))
	for i, k := range lv.keys {
		vals[i] = labels[k]
	}
	lv.vec.WithLabelValues(vals...).Add(float64(delta))
}

// SetGauge 设置无标签 gauge。
func (r *Registry) SetGauge(name, help string, v int64) {
	r.mu.Lock()
	g, ok := r.gauges[name]
	if !ok {
		g = prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: helpOrName(help, name)})
		r.reg.MustRegister(g)
		r.gauges[name] = g
	}
	r.mu.Unlock()
	g.Set(float64(v))
}

// ObserveSyncRun 记录一次同步运行结果并刷新最近运行时间戳。
func (r *Registry) ObserveSyncRun(status string) {
	r.ObserveSyncRunWithTask("", status)
}

// ObserveSyncRunWithTask 按任务+状态记录(任务数有界,不会打爆基数)。
func (r *Registry) ObserveSyncRunWithTask(taskKey, status string) {
	r.AddLabeled("sync_runs_total", "Sync runs by status",
		map[string]string{"status": status, "task": taskKey}, 1)
	r.SetGauge("sync_last_run_timestamp_seconds",
		"Unix timestamp of last observed sync run", r.now().Unix())
}

// WritePrometheus 渲染 Prometheus 文本格式(测试与诊断用;/metrics 端点走 hertz/metrics)。
func (r *Registry) WritePrometheus() string {
	mfs, err := r.reg.Gather()
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			break
		}
	}
	return buf.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// ObserveSyncRunWithTask 记录到默认注册表(带 task 标签)。
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
