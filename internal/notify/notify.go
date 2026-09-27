package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// RunEvent 一次同步运行的结束事件。
type RunEvent struct {
	TaskKey   string
	TaskName  string
	RunID     uint
	Status    string // success | failed
	Trigger   string
	Error     string
	ErrorType string
	Duration  time.Duration
	EndAt     time.Time
}

// Notifier 多通道通知器。
type Notifier struct {
	cfg    *Config
	client *http.Client
}

// New 创建通知器;cfg 为空时返回可安全调用的空实现。
func New(cfg *Config) *Notifier {
	if cfg == nil {
		cfg = &Config{}
	}
	return &Notifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyRun 异步派发运行结束通知(不阻塞调用方)。
func (n *Notifier) NotifyRun(ev *RunEvent) {
	if n == nil || ev == nil || !n.cfg.Enabled() {
		return
	}
	if n.cfg.OnlyFail && ev.Status == "success" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		n.pushAll(ctx, ev)
	}()
}

func (n *Notifier) pushAll(ctx context.Context, ev *RunEvent) {
	for i := range n.cfg.Ntfy {
		cfg := &n.cfg.Ntfy[i]
		if !wantEvent(cfg.Events, ev.Status) {
			continue
		}
		if err := n.sendNtfy(ctx, cfg, ev); err != nil {
			slog.Warn("notify ntfy failed", "error", err, "url", cfg.URL)
		}
	}
	for i := range n.cfg.Gotify {
		cfg := &n.cfg.Gotify[i]
		if !wantEvent(cfg.Events, ev.Status) {
			continue
		}
		if err := n.sendGotify(ctx, cfg, ev); err != nil {
			slog.Warn("notify gotify failed", "error", err, "url", cfg.URL)
		}
	}
}

// wantEvent events 为空表示 success+failed 都要。
func wantEvent(events []string, status string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if strings.EqualFold(e, status) {
			return true
		}
	}
	return false
}

func (n *Notifier) sendNtfy(ctx context.Context, cfg *NtfyConfig, ev *RunEvent) error {
	url := cfg.URL
	if cfg.Topic != "" {
		url = strings.TrimRight(url, "/") + "/" + cfg.Topic
	}
	body := formatBody(ev)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", fmt.Sprintf("GitFerry %s: %s", ev.Status, ev.TaskName))
	req.Header.Set("Priority", priorityOf(ev.Status))
	req.Header.Set("Tags", tagsOf(ev.Status))
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	} else if cfg.User != "" {
		req.SetBasicAuth(cfg.User, cfg.Pass)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return n.do(req)
}

func (n *Notifier) sendGotify(ctx context.Context, cfg *GotifyConfig, ev *RunEvent) error {
	payload := map[string]any{
		"title":    fmt.Sprintf("GitFerry %s: %s", ev.Status, ev.TaskName),
		"message":  formatBody(ev),
		"priority": gotifyPriority(ev.Status),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := strings.TrimRight(cfg.URL, "/") + "/message?token=" + cfg.Token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return n.do(req)
}

func (n *Notifier) do(req *http.Request) error {
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify status %d", resp.StatusCode)
	}
	return nil
}

func formatBody(ev *RunEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "任务: %s (%s)\n", ev.TaskName, ev.TaskKey)
	fmt.Fprintf(&b, "状态: %s  触发: %s\n", ev.Status, ev.Trigger)
	fmt.Fprintf(&b, "耗时: %s\n", ev.Duration.Round(time.Second))
	if ev.Error != "" {
		fmt.Fprintf(&b, "错误: %s\n", ev.Error)
	}
	fmt.Fprintf(&b, "run_id: %d", ev.RunID)
	return b.String()
}

func priorityOf(status string) string {
	if status == "failed" {
		return "high"
	}
	return "default"
}

func tagsOf(status string) string {
	if status == "failed" {
		return "x,rotating_light"
	}
	return "white_check_mark"
}

func gotifyPriority(status string) int {
	if status == "failed" {
		return 8
	}
	return 5
}
