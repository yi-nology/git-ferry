package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yi-nology/git-sync-service/internal/lock"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// releaseFunc 释放 Acquire 拿到的执行权(任务互斥锁 + 全局并发槽),由调用方 defer。
type releaseFunc func()

// concurrencyGuard 统一封装“同 taskKey 互斥 + 全局并发上限”。
// 单实例(无 redis)用 localGuard;多实例(配 redis)用 redisGuard。
type concurrencyGuard interface {
	// Acquire 尝试获取 taskKey 的执行权 + 一个全局并发槽。
	// 成功返回 release;taskKey 已在跑返回 ErrTaskRunning;全局满返回 ErrTooManyConcurrent。
	Acquire(ctx context.Context, taskKey string) (releaseFunc, error)
	// TryCronLock 尝试获取 cron 分布式锁(短 TTL),防止多实例重复触发。
	// 本地模式始终返回 true;Redis 模式用 SetNX 实现。
	TryCronLock(ctx context.Context, taskKey string, ttl time.Duration) (bool, error)
	// Ping 检查后端连通性(本地实现始终返回 nil)。
	Ping() error
	Close() error
}

const (
	taskLockTTL    = 30 * time.Second // 任务锁 TTL,watchdog 周期续期
	taskLockKeyPfx = "task:"          // redis 任务锁 key 前缀
	semSlotKey     = "sync-tasks"     // 全局并发信号量 key
	semSlotTTL     = 30 * time.Second // 信号量槽 TTL,watchdog 续期
	renewInterval  = 10 * time.Second // watchdog 续期间隔(< TTL/2)
)

// ---------------- 本地(进程内)实现 ----------------

// localGuard 单实例并发控制:sync.Map 做 taskKey 互斥,buffered channel 做全局并发上限。
type localGuard struct {
	runningTasks sync.Map
	sem          chan struct{}
}

func newLocalGuard(maxConcurrent int) *localGuard {
	if maxConcurrent <= 0 {
		maxConcurrent = model.DefaultMaxConcurrent
	}
	return &localGuard{sem: make(chan struct{}, maxConcurrent)}
}

func (g *localGuard) Acquire(_ context.Context, taskKey string) (releaseFunc, error) {
	select {
	case g.sem <- struct{}{}:
	default:
		return nil, ErrTooManyConcurrent
	}
	// 只有成功 store 才注册 Delete,避免误删别的执行流的标记
	if _, loaded := g.runningTasks.LoadOrStore(taskKey, struct{}{}); loaded {
		<-g.sem
		return nil, ErrTaskRunning
	}
	return func() {
		g.runningTasks.Delete(taskKey)
		<-g.sem
	}, nil
}

func (g *localGuard) Ping() error  { return nil }
func (g *localGuard) Close() error { return nil }
func (g *localGuard) TryCronLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil // 本地模式始终成功
}

// ---------------- 分布式(redis)实现 ----------------

// redisGuard 多实例并发控制:RedisLock 做 taskKey 互斥,ZSET 信号量做全局并发上限,
// 均带 watchdog 续期。进程崩溃后锁/槽在 TTL 内自动释放。
type redisGuard struct {
	rlock *lock.RedisLock
	sem   *lock.Semaphore
}

func newRedisGuard(addr, password string, db, maxConcurrent int, poolOpts lock.RedisPoolOptions) (*redisGuard, error) {
	if maxConcurrent <= 0 {
		maxConcurrent = model.DefaultMaxConcurrent
	}
	rl := lock.NewRedisLock(addr, password, db, lock.WithPoolOptions(poolOpts))
	if err := rl.Ping(context.Background()); err != nil {
		_ = rl.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	sem := lock.NewSemaphore(rl.Client(), semSlotKey, maxConcurrent, semSlotTTL)
	return &redisGuard{rlock: rl, sem: sem}, nil
}

func (g *redisGuard) Acquire(ctx context.Context, taskKey string) (releaseFunc, error) {
	// 1) 全局并发槽
	semID := uuid.NewString()
	ok, err := g.sem.Acquire(ctx, semID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTooManyConcurrent
	}

	// 2) 同 taskKey 互斥锁
	lockKey := taskLockKeyPfx + taskKey
	locked, value, err := g.rlock.LockWithTTL(ctx, lockKey, taskLockTTL)
	if err != nil {
		_ = g.sem.Release(ctx, semID)
		return nil, err
	}
	if !locked {
		_ = g.sem.Release(ctx, semID)
		return nil, ErrTaskRunning
	}

	// 3) watchdog 续期锁与信号量槽,避免长任务期间过期被别的实例抢走
	stop := make(chan struct{})
	go g.watchdog(lockKey, value, semID, stop) //nolint:gosec // watchdog goroutine 需要独立于请求生命周期运行

	return func() {
		close(stop)
		// 用带超时的独立 context 释放,避免 Redis 不可响应时 goroutine 永久挂死
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		if err := g.rlock.UnlockWithValue(releaseCtx, lockKey, value); err != nil {
			slog.Warn("release: failed to unlock", "lockKey", lockKey, "error", err)
		}
		if err := g.sem.Release(releaseCtx, semID); err != nil {
			slog.Warn("release: failed to release semaphore slot", "semID", semID, "error", err)
		}
	}, nil
}

// watchdog 周期性续期持有的锁与信号量槽。stop 关闭即退出。
// 使用 context.Background() 是有意为之：watchdog 作为后台 goroutine 需要在请求上下文取消后继续运行，
// 直到 stop 通道被关闭。这是后台任务续期的标准模式。
func (g *redisGuard) watchdog(lockKey, value, semID string, stop <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("watchdog goroutine panic recovered", "lockKey", lockKey, "panic", fmt.Sprintf("%v", r))
		}
	}()
	t := time.NewTicker(renewInterval)
	defer t.Stop()
	ctx := context.Background() //nolint:gosec // watchdog goroutine intentionally uses background context
	failures := 0
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if _, err := g.rlock.ExtendLockWithValue(ctx, lockKey, value, taskLockTTL); err != nil {
				failures++
				if failures%3 == 1 { // 每 3 次失败记一条日志,避免刷屏
					slog.Warn("watchdog: failed to extend lock", "lockKey", lockKey, "error", err, "consecutive_failures", failures)
				}
			} else {
				failures = 0
				if err := g.sem.Renew(ctx, semID); err != nil {
					slog.Warn("watchdog: failed to renew semaphore slot", "semID", semID, "error", err)
				}
			}
		}
	}
}

func (g *redisGuard) Ping() error {
	return g.rlock.Ping(context.Background())
}

// TryCronLock 用 Redis SetNX 获取短 TTL 的 cron 锁,防止多实例重复触发。
func (g *redisGuard) TryCronLock(ctx context.Context, taskKey string, ttl time.Duration) (bool, error) {
	return g.rlock.TryLockWithTTL(ctx, "cron:"+taskKey, ttl)
}

func (g *redisGuard) Close() error {
	return g.rlock.Close()
}

// newGuard 按是否配置 redis 选择实现。配了 redis 用分布式,否则用进程内。
func newGuard(redisAddr, redisPassword string, redisDB, maxConcurrent int, poolOpts lock.RedisPoolOptions) (concurrencyGuard, error) {
	if redisAddr != "" {
		rg, err := newRedisGuard(redisAddr, redisPassword, redisDB, maxConcurrent, poolOpts)
		if err != nil {
			// redis 连不上不应让整个服务起不来:回退到本地模式并告警
			return nil, fmt.Errorf("init redis guard failed (check redis config or remove redis.addr to use single-instance mode): %w", err)
		}
		return rg, nil
	}
	return newLocalGuard(maxConcurrent), nil
}
