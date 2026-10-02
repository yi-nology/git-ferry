// Package runwatch 把「运行结束」转成通知/指标/心跳与失败自动补偿。
//
// 事件源两种模式：
//   - event（默认）：订阅 core 的 SubscribeRuns 完成事件，不再轮询；
//     core 在 RunTaskAsync / RunTaskWithTrigger 收尾处广播（cron/webhook/手动全覆盖）。
//   - poll：轮询 ListHistory 兜底（旧实现，可在 yaml runwatch.mode 切回），
//     首轮只建水位不发通知，避免启动把历史全喷一遍。
//
// 失败自动补偿的次数与冷却判定在 core RetryTracker（Service.AutoRetry），
// 壳层只负责「判定放行 → RunTaskAsync(auto_retry) + 通知」。
package runwatch

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/metrics"
	"github.com/yi-nology/git-ferry/internal/notify"
)

// ModeEvent / ModePoll 事件源模式。
const (
	ModeEvent = "event"
	ModePoll  = "poll"
)

// SyncService 观察与补偿所需的最小 Service 面向接口。
type SyncService interface {
	ListTasks(ctx context.Context, repoKey string, offset, limit int) ([]*corebridge.SyncTask, int64, error)
	ListHistory(ctx context.Context, taskKey string, offset, limit int) ([]*corebridge.SyncRun, int64, error)
	RunTaskAsync(taskKey, trigger string, webhookEventID *uint) error
	// SubscribeRuns 订阅 core 运行完成事件，返回取消函数。
	SubscribeRuns(fn func(corebridge.RunEvent)) func()
	// AutoRetry 失败自动补偿的策略计数器。
	AutoRetry() *corebridge.RetryTracker
}

// RetryConfig 失败自动补偿（转成 core AutoRetryPolicy）。
type RetryConfig struct {
	// MaxAutoRetries 同一运行失败后的自动重跑次数;0=不自动重跑
	MaxAutoRetries int `yaml:"max_auto_retries"`
	// CooldownMinutes 两次自动重跑之间的冷却
	CooldownMinutes int `yaml:"cooldown_minutes"`
}

// policy 转成 core 策略。
func (r RetryConfig) policy() corebridge.AutoRetryPolicy {
	cooldown := time.Duration(r.CooldownMinutes) * time.Minute
	if r.CooldownMinutes <= 0 {
		cooldown = 5 * time.Minute
	}
	return corebridge.AutoRetryPolicy{MaxAutoRetries: r.MaxAutoRetries, Cooldown: cooldown}
}

// Config 观察器配置。
type Config struct {
	// Mode event(默认) | poll
	Mode            string      `yaml:"mode"`
	IntervalSeconds int         `yaml:"interval_seconds"`
	HistoryLimit    int         `yaml:"history_limit"`
	Retry           RetryConfig `yaml:"retry"`
}

// Watcher 运行观察器。
type Watcher struct {
	svc      SyncService
	notifier *notify.Notifier
	hb       *notify.Heartbeat
	cfg      Config

	mu   sync.Mutex
	seen map[uint]bool
	// firstPass 首轮只建水位不发通知,避免启动把历史全喷一遍（仅 poll 模式）
	firstPass bool
	// taskNames run.TaskKey -> 任务名(通知更友好)
	taskNames map[string]string
}

// New 创建观察器。
func New(svc SyncService, n *notify.Notifier, hb *notify.Heartbeat, cfg Config) *Watcher {
	if cfg.Mode == "" {
		cfg.Mode = ModeEvent
	}
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 30
	}
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 10
	}
	if cfg.Retry.CooldownMinutes <= 0 {
		cfg.Retry.CooldownMinutes = 5
	}
	return &Watcher{
		svc: svc, notifier: n, hb: hb, cfg: cfg,
		seen: map[uint]bool{}, firstPass: true, taskNames: map[string]string{},
	}
}

// Start 启动观察;ctx 取消即停。
func (w *Watcher) Start(ctx context.Context) {
	if w.cfg.Mode == ModeEvent {
		cancel := w.svc.SubscribeRuns(func(ev corebridge.RunEvent) {
			w.onRunEvent(ctx, &ev)
		})
		go func() {
			<-ctx.Done()
			cancel()
		}()
		slog.Info("run watcher started",
			"mode", ModeEvent, "max_auto_retries", w.cfg.Retry.MaxAutoRetries)
		return
	}

	go func() {
		t := time.NewTicker(time.Duration(w.cfg.IntervalSeconds) * time.Second)
		defer t.Stop()
		w.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx)
			}
		}
	}()
	slog.Info("run watcher started",
		"mode", ModePoll,
		"interval_seconds", w.cfg.IntervalSeconds,
		"max_auto_retries", w.cfg.Retry.MaxAutoRetries)
}

// onRunEvent 完成事件处理：指标 → 心跳 → 通知 → 失败自动补偿。
func (w *Watcher) onRunEvent(ctx context.Context, ev *corebridge.RunEvent) {
	run := ev.Run
	if run == nil {
		return // CreateRun 失败等，没有可播报的运行
	}
	if run.Status != "success" && run.Status != "failed" {
		return
	}
	w.emit(ctx, run)
}

// emit 播报一次已完成的运行（事件/轮询两条路径共用）。
func (w *Watcher) emit(ctx context.Context, run *corebridge.SyncRun) {
	metrics.ObserveSyncRunWithTask(run.TaskKey, run.Status)
	failed := run.Status == "failed"
	if w.hb != nil {
		w.hb.MarkResult(failed)
	}
	endAt := time.Now()
	if run.EndTime != nil {
		endAt = *run.EndTime
	}
	w.notifier.NotifyRun(&notify.RunEvent{
		TaskKey:   run.TaskKey,
		TaskName:  w.taskName(ctx, run.TaskKey),
		RunID:     run.ID,
		Status:    run.Status,
		Trigger:   run.TriggerSource,
		Error:     run.ErrorMessage,
		ErrorType: run.ErrorType,
		Duration:  time.Duration(run.DurationMs) * time.Millisecond,
		EndAt:     endAt,
	})

	if failed {
		w.maybeAutoRetry(ctx, run)
	}
}

// taskName 任务名（事件模式下按需查一次并缓存；轮询模式在扫描时已填充）。
func (w *Watcher) taskName(ctx context.Context, key string) string {
	w.mu.Lock()
	name := w.taskNames[key]
	w.mu.Unlock()
	if name != "" {
		return name
	}
	// 可选能力：实现方（真实 Service）可查单个任务；测试桩可不实现
	getter, ok := w.svc.(taskGetter)
	if !ok {
		return ""
	}
	if task, err := getter.GetTask(ctx, key); err == nil && task != nil && task.Name != "" {
		w.mu.Lock()
		w.taskNames[key] = task.Name
		w.mu.Unlock()
		return task.Name
	}
	return ""
}

// taskGetter 可选能力：查单个任务（轮询模式的 fake 可不实现）。
type taskGetter interface {
	GetTask(ctx context.Context, key string) (*corebridge.SyncTask, error)
}

// maybeAutoRetry 询问 core 策略，放行则触发一次 auto_retry。
func (w *Watcher) maybeAutoRetry(ctx context.Context, run *corebridge.SyncRun) {
	d := w.svc.AutoRetry().ShouldRetry(run, w.cfg.Retry.policy())
	if !d.Retry {
		return
	}
	slog.Info("auto retry failed sync run",
		"run_id", run.ID, "task", run.TaskKey, "attempt", d.Attempt)
	if err := w.svc.RunTaskAsync(run.TaskKey, "auto_retry", nil); err != nil {
		slog.Warn("auto retry failed", "run_id", run.ID, "error", err, "reason", d.Reason)
	}
}

// Tick 导出供测试:扫描一轮（仅 poll 模式使用）。
func (w *Watcher) Tick(ctx context.Context) { w.tick(ctx) }

func (w *Watcher) tick(ctx context.Context) {
	runs, err := w.collectRecent(ctx)
	if err != nil {
		slog.Warn("run watch collect failed", "error", err)
		return
	}
	w.pruneMaps(runs)
	sortRunsByID(runs)

	w.mu.Lock()
	first := w.firstPass
	w.firstPass = false
	w.mu.Unlock()

	for _, run := range runs {
		if run.Status != "success" && run.Status != "failed" {
			continue
		}
		w.mu.Lock()
		already := w.seen[run.ID]
		if !already {
			w.seen[run.ID] = true
		}
		w.mu.Unlock()

		if first || already {
			continue // 首轮只建水位;已见过跳过
		}
		w.emit(ctx, run)
	}
}

func (w *Watcher) collectRecent(ctx context.Context) ([]*corebridge.SyncRun, error) {
	const pageSize = 50
	var out []*corebridge.SyncRun
	offset := 0
	for {
		tasks, total, err := w.svc.ListTasks(ctx, "", offset, pageSize)
		if err != nil {
			return out, err
		}
		for _, t := range tasks {
			if t.Name != "" {
				w.mu.Lock()
				w.taskNames[t.Key] = t.Name
				w.mu.Unlock()
			}
			runs, _, err := w.svc.ListHistory(ctx, t.Key, 0, w.cfg.HistoryLimit)
			if err != nil {
				continue // 单任务失败不拖垮整轮
			}
			out = append(out, runs...)
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return out, nil
}

// pruneMaps 防止 seen/taskNames 随 run ID 无限增长。
// 只保留「本轮扫描到的 ID + 最近 1000 个」,旧 ID 丢弃。
// run ID 单调递增,过期条目不会再被查询命中。
func (w *Watcher) pruneMaps(current []*corebridge.SyncRun) {
	const maxKeep = 1000
	live := make(map[uint]bool, len(current))
	for _, r := range current {
		live[r.ID] = true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.seen) <= maxKeep {
		return
	}
	for id := range w.seen {
		if !live[id] {
			delete(w.seen, id)
		}
	}
	// 超限时再砍:按 ID 升序删最老的
	if len(w.seen) > maxKeep {
		ids := make([]uint, 0, len(w.seen))
		for id := range w.seen {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids[:len(ids)-maxKeep] {
			delete(w.seen, id)
		}
	}
}

func sortRunsByID(runs []*corebridge.SyncRun) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].ID < runs[j-1].ID; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
}
