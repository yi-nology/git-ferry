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
		w.retries[i] = 1
	}
	current := []*corebridge.SyncRun{{ID: 1200, TaskKey: "t", Status: "success"}}
	w.pruneMaps(current)
	w.mu.Lock()
	n := len(w.seen)
	w.mu.Unlock()
	assert.LessOrEqual(t, n, 1000)
	assert.True(t, w.seen[1200])
}
