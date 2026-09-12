package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// cleanupResult 用于并行 cleanup goroutine 的结果传递。
type cleanupResult struct {
	count int64
	err   error
}

// recoverCleanup 用于 defer recover,将 panic 转为 error 保留在 result 中,
// 防止单个 cleanup goroutine panic 导致整个进程崩溃。
func recoverCleanup(name string, r *cleanupResult) {
	if v := recover(); v != nil {
		r.err = fmt.Errorf("cleanup %s panic: %v", name, v)
		slog.Error("cleanup goroutine panic recovered", "task", name, "panic", fmt.Sprintf("%v", v))
	}
}

func (s *Service) CleanupOldData(ctx context.Context, maxAge time.Duration) (events, runs, steps int64, err error) {
	// 三张表独立清理,互不依赖,并行执行缩短耗时。
	ch := make(chan cleanupResult, 3)

	go func() {
		var r cleanupResult
		defer func() { recoverCleanup("CleanupOldEvents", &r); ch <- r }()
		r.count, r.err = s.webhooks.CleanupOldEvents(ctx, maxAge)
	}()
	go func() {
		var r cleanupResult
		defer func() { recoverCleanup("CleanupOldRuns", &r); ch <- r }()
		r.count, r.err = s.tasks.CleanupOldRuns(ctx, maxAge)
	}()
	go func() {
		var r cleanupResult
		defer func() { recoverCleanup("CleanupOldRunSteps", &r); ch <- r }()
		r.count, r.err = s.tasks.CleanupOldRunSteps(ctx, maxAge)
	}()

	r1, r2, r3 := <-ch, <-ch, <-ch
	events, runs, steps = r1.count, r2.count, r3.count

	// 聚合所有错误而非只保留最后一个,方便排查多个表同时出问题的情况
	var errs []error
	for _, r := range []cleanupResult{r1, r2, r3} {
		if r.err != nil {
			errs = append(errs, r.err)
		}
	}
	slog.Info("data cleanup completed", "events_deleted", events, "runs_deleted", runs, "steps_deleted", steps)
	return events, runs, steps, errors.Join(errs...)
}

func (s *Service) cleanupTriggerTimes() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.lastTriggerTime.Range(func(key, value interface{}) bool {
				if t, ok := value.(time.Time); ok && now.Sub(t) > 1*time.Hour {
					s.lastTriggerTime.Delete(key)
				}
				return true
			})
		case <-s.cleanupDone:
			return
		}
	}
}
