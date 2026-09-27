package notify

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Heartbeat 按「最近是否有失败」分路打点。
// healthchecks.io 语义:成功 ping 保持绿,失败 ping 标红。
type Heartbeat struct {
	cfg    *HeartbeatConfig
	client *http.Client
	// lastFailUnix 最近一次观察到失败的时间(0=尚未失败)
	lastFailUnix int64
}

// NewHeartbeat 创建心跳器。
func NewHeartbeat(cfg *HeartbeatConfig) *Heartbeat {
	if cfg == nil {
		return nil
	}
	return &Heartbeat{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// MarkResult 记录一次运行结果,并立即向对应分路打点。
func (h *Heartbeat) MarkResult(failed bool) {
	if h == nil {
		return
	}
	if failed {
		h.lastFailUnix = time.Now().Unix()
		h.pingAll(h.cfg.FailURLs, "fail")
		// 失败也 ping 成功通道,让监视方知道进程仍存活
		h.pingAll(h.cfg.SuccessURLs, "")
		return
	}
	// 成功:仅当没有悬挂失败时 ping 成功通道
	if h.lastFailUnix == 0 {
		h.pingAll(h.cfg.SuccessURLs, "")
	}
}

// Start 周期心跳(IntervalSeconds>0 时)。ctx 取消即停。
func (h *Heartbeat) Start(ctx context.Context) {
	if h == nil || h.cfg.IntervalSeconds <= 0 {
		return
	}
	interval := time.Duration(h.cfg.IntervalSeconds) * time.Second
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if h.lastFailUnix != 0 && time.Since(time.Unix(h.lastFailUnix, 0)) > 24*time.Hour {
					// 失败悬挂超 24h,重新允许成功分路
					h.lastFailUnix = 0
				}
				if h.lastFailUnix == 0 {
					h.pingAll(h.cfg.SuccessURLs, "")
				}
			}
		}
	}()
}

func (h *Heartbeat) pingAll(urls []string, suffix string) {
	for _, u := range urls {
		target := u
		if suffix != "" && target != "" {
			// healthchecks.io: /ping/<uuid>/fail
			target = trimSlash(target) + "/" + suffix
		}
		if err := h.ping(target); err != nil {
			slog.Warn("heartbeat ping failed", "url", target, "error", err)
		}
	}
}

func (h *Heartbeat) ping(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

func trimSlash(s string) string {
	for s != "" && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
