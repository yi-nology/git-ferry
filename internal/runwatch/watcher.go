// Package runwatch 轮询同步执行历史,把「运行结束」事件转成通知/指标/失败补偿。
//
// 为什么是轮询而非 core 钩子:core.RunTaskAsync 异步执行且不暴露完成回调,
// 壳层用 ListHistory 做事件源,不改 core 也能闭环。seen 水位按 run ID 去重。
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

// SyncService 观察与补偿所需的最小 Service 面向接口。
type SyncService interface {
	ListTasks(ctx context.Context, repoKey string, offset, limit int) ([]*corebridge.SyncTask, int64, error)
	ListHistory(ctx context.Context, taskKey string, offset, limit int) ([]*corebridge.SyncRun, int64, error)
	RunTaskAsync(taskKey, trigger string, webhookEventID *uint) error
}

// RetryConfig 失败自动补偿。
type RetryConfig struct {
	// MaxAutoRetries 同一运行失败后的自动重跑次数;0=不自动重跑
	MaxAutoRetries int `yaml:"max_auto_retries"`
	// CooldownMinutes 两次自动重跑之间的冷却
	CooldownMinutes int `yaml:"cooldown_minutes"`
}

// Config 观察器配置。
type Config struct {
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

	mu       sync.Mutex
	seen     map[uint]bool
	retries  map[uint]int       // runID -> 已自动重跑次数
	lastAuto map[uint]time.Time // runID -> 上次自动重跑时间
	// firstPass 首轮只建水位不发通知,避免启动把历史全喷一遍
	firstPass bool
	// taskNames run.TaskKey -> 任务名(通知更友好)
	taskNames map[string]string
}

// New 创建观察器。
func New(svc SyncService, n *notify.Notifier, hb *notify.Heartbeat, cfg Config) *Watcher {
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
		seen: map[uint]bool{}, retries: map[uint]int{}, lastAuto: map[uint]time.Time{},
		firstPass: true, taskNames: map[string]string{},
	}
}

// Start 启动轮询;ctx 取消即停。
func (w *Watcher) Start(ctx context.Context) {
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
		"interval_seconds", w.cfg.IntervalSeconds,
		"max_auto_retries", w.cfg.Retry.MaxAutoRetries)
}

// Tick 导出供测试:扫描一轮。
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
		name := w.taskNames[run.TaskKey]
		w.mu.Unlock()

		if first || already {
			continue // 首轮只建水位;已见过跳过
		}

		metrics.ObserveSyncRun(run.Status)
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
			TaskName:  name,
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
}

func (w *Watcher) maybeAutoRetry(ctx context.Context, run *corebridge.SyncRun) {
	if w.cfg.Retry.MaxAutoRetries <= 0 {
		return
	}
	w.mu.Lock()
	n := w.retries[run.ID]
	last := w.lastAuto[run.ID]
	if n >= w.cfg.Retry.MaxAutoRetries {
		w.mu.Unlock()
		return
	}
	cooldown := time.Duration(w.cfg.Retry.CooldownMinutes) * time.Minute
	if !last.IsZero() && time.Since(last) < cooldown {
		w.mu.Unlock()
		return
	}
	w.retries[run.ID] = n + 1
	w.lastAuto[run.ID] = time.Now()
	w.mu.Unlock()

	slog.Info("auto retry failed sync run",
		"run_id", run.ID, "task", run.TaskKey, "attempt", n+1)
	if err := w.svc.RunTaskAsync(run.TaskKey, "auto_retry", nil); err != nil {
		slog.Warn("auto retry failed", "run_id", run.ID, "error", err)
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
			w.mu.Lock()
			if t.Name != "" {
				w.taskNames[t.Key] = t.Name
			}
			w.mu.Unlock()
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

// pruneMaps 防止 seen/retries/lastAuto 随 run ID 无限增长。
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
		// 仍需清掉不在 live 且过老的,但小规模先只删非 live 超限的情况
		return
	}
	for id := range w.seen {
		if !live[id] {
			delete(w.seen, id)
			delete(w.retries, id)
			delete(w.lastAuto, id)
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
			delete(w.retries, id)
			delete(w.lastAuto, id)
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
