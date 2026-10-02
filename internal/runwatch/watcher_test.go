package runwatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

type fakeSvc struct {
	mu     sync.Mutex
	tasks  []*corebridge.SyncTask
	runs   map[string][]*corebridge.SyncRun
	called []string
	retry  *corebridge.RetryTracker
}

func (f *fakeSvc) ListTasks(context.Context, string, int, int) ([]*corebridge.SyncTask, int64, error) {
	return f.tasks, int64(len(f.tasks)), nil
}

func (f *fakeSvc) ListHistory(_ context.Context, key string, _, _ int) ([]*corebridge.SyncRun, int64, error) {
	rs := f.runs[key]
	return rs, int64(len(rs)), nil
}

func (f *fakeSvc) RunTaskAsync(taskKey, trigger string, _ *uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = append(f.called, taskKey+":"+trigger)
	return nil
}

func (f *fakeSvc) SubscribeRuns(func(corebridge.RunEvent)) func() { return func() {} }

func (f *fakeSvc) AutoRetry() *corebridge.RetryTracker {
	if f.retry == nil {
		f.retry = corebridge.NewRetryTracker(0)
	}
	return f.retry
}

func TestWatcher_DetectsNewFailedRunAndRetries(t *testing.T) {
	end := time.Now()
	svc := &fakeSvc{
		tasks: []*corebridge.SyncTask{{Key: "t1", Name: "T1"}},
		runs: map[string][]*corebridge.SyncRun{
			"t1": {{
				ID: 1, TaskKey: "t1", Status: "failed",
				TriggerSource: "manual", ErrorMessage: "boom",
				DurationMs: 1200, EndTime: &end,
			}},
		},
	}
	w := New(svc, nil, nil, Config{
		IntervalSeconds: 1, HistoryLimit: 5,
		Retry: RetryConfig{MaxAutoRetries: 1, CooldownMinutes: 1},
	})

	// 首轮只建水位
	w.Tick(context.Background())
	assert.Empty(t, svc.called)

	// 新增一条失败 → 观察到并自动重试
	svc.runs["t1"] = append(svc.runs["t1"], &corebridge.SyncRun{
		ID: 2, TaskKey: "t1", Status: "failed",
		TriggerSource: "cron", ErrorMessage: "boom2", EndTime: &end,
	})
	w.Tick(context.Background())

	svc.mu.Lock()
	got := append([]string(nil), svc.called...)
	svc.mu.Unlock()
	require.Equal(t, []string{"t1:auto_retry"}, got)
}

func TestWatcher_SkipsAlreadySeen(t *testing.T) {
	end := time.Now()
	svc := &fakeSvc{
		tasks: []*corebridge.SyncTask{{Key: "t1"}},
		runs: map[string][]*corebridge.SyncRun{
			"t1": {{ID: 7, TaskKey: "t1", Status: "success", EndTime: &end}},
		},
	}
	w := New(svc, nil, nil, Config{Retry: RetryConfig{MaxAutoRetries: 3}})
	w.Tick(context.Background()) // first pass marks seen
	w.Tick(context.Background()) // should not reprocess
	svc.mu.Lock()
	assert.Empty(t, svc.called)
	svc.mu.Unlock()
}

func TestWatcher_RetryCooldown(t *testing.T) {
	end := time.Now()
	svc := &fakeSvc{
		tasks: []*corebridge.SyncTask{{Key: "t1"}},
		runs: map[string][]*corebridge.SyncRun{
			"t1": {{ID: 3, TaskKey: "t1", Status: "failed", EndTime: &end}},
		},
	}
	w := New(svc, nil, nil, Config{
		Retry: RetryConfig{MaxAutoRetries: 5, CooldownMinutes: 10},
	})
	w.Tick(context.Background()) // watermark
	// 同一条 failed 一直存在,但已 seen,不会重试
	w.Tick(context.Background())
	svc.mu.Lock()
	assert.Empty(t, svc.called)
	svc.mu.Unlock()
}

func TestWatcher_PruneMaps(t *testing.T) {
	svc := &fakeSvc{tasks: nil, runs: map[string][]*corebridge.SyncRun{}}
	w := New(svc, nil, nil, Config{})
	// 塞 1200 条
	for i := uint(1); i <= 1200; i++ {
		w.seen[i] = true
	}
	current := []*corebridge.SyncRun{{ID: 1200, TaskKey: "t", Status: "success"}}
	w.pruneMaps(current)
	w.mu.Lock()
	n := len(w.seen)
	w.mu.Unlock()
	assert.LessOrEqual(t, n, 1000)
	assert.True(t, w.seen[1200])
}

// ===== 事件模式 =====

type eventSvc struct {
	fakeSvc
	mu  sync.Mutex
	sub func(corebridge.RunEvent)
}

func (e *eventSvc) SubscribeRuns(fn func(corebridge.RunEvent)) func() {
	e.mu.Lock()
	e.sub = fn
	e.mu.Unlock()
	return func() {}
}

func (e *eventSvc) handler() func(corebridge.RunEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sub
}

// TestWatcher_EventMode_Retries 完成事件直达：失败触发 auto_retry，成功不触发。
func TestWatcher_EventMode_Retries(t *testing.T) {
	svc := &eventSvc{fakeSvc: fakeSvc{tasks: []*corebridge.SyncTask{{Key: "t1", Name: "T1"}}}}
	w := New(svc, nil, nil, Config{
		Mode:  ModeEvent,
		Retry: RetryConfig{MaxAutoRetries: 1, CooldownMinutes: 1},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	require.Eventually(t, func() bool { return svc.handler() != nil }, time.Second, 5*time.Millisecond,
		"Start 后应完成订阅")
	handler := svc.handler()

	end := time.Now()
	// 成功运行：不重跑
	handler(corebridge.RunEvent{TaskKey: "t1", Run: &corebridge.SyncRun{
		ID: 1, TaskKey: "t1", Status: "success", EndTime: &end,
	}})
	// CreateRun 失败类事件（Run=nil）：直接忽略，不 panic
	handler(corebridge.RunEvent{TaskKey: "t1", Run: nil})
	time.Sleep(30 * time.Millisecond)
	svc.mu.Lock()
	assert.Empty(t, svc.called, "成功/空事件不应触发重跑")
	svc.mu.Unlock()

	// 失败运行：触发一次 auto_retry
	handler(corebridge.RunEvent{TaskKey: "t1", Run: &corebridge.SyncRun{
		ID: 2, TaskKey: "t1", Status: "failed", ErrorMessage: "boom", EndTime: &end,
	}})
	require.Eventually(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.called) == 1 && svc.called[0] == "t1:auto_retry"
	}, time.Second, 5*time.Millisecond, "失败事件应触发 auto_retry")

	// 同一 run 冷却期内不再重跑
	handler(corebridge.RunEvent{TaskKey: "t1", Run: &corebridge.SyncRun{
		ID: 2, TaskKey: "t1", Status: "failed", ErrorMessage: "boom", EndTime: &end,
	}})
	time.Sleep(50 * time.Millisecond)
	svc.mu.Lock()
	assert.Len(t, svc.called, 1, "冷却期内不应重复重跑")
	svc.mu.Unlock()
}

// TestWatcher_PollModeStillWorks 显式指定 poll 模式仍走轮询路径。
func TestWatcher_PollModeStillWorks(t *testing.T) {
	end := time.Now()
	svc := &fakeSvc{
		tasks: []*corebridge.SyncTask{{Key: "t1"}},
		runs: map[string][]*corebridge.SyncRun{
			"t1": {{ID: 11, TaskKey: "t1", Status: "success", EndTime: &end}},
		},
	}
	w := New(svc, nil, nil, Config{Mode: ModePoll, Retry: RetryConfig{MaxAutoRetries: 1}})
	w.Tick(context.Background()) // 水位
	svc.runs["t1"] = append(svc.runs["t1"], &corebridge.SyncRun{
		ID: 12, TaskKey: "t1", Status: "success", EndTime: &end,
	})
	w.Tick(context.Background()) // 新事件：成功不重跑但会被观测（called 仍空）
	svc.mu.Lock()
	assert.Empty(t, svc.called)
	svc.mu.Unlock()
}
