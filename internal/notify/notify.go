package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oekit/concurrency/async"
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
	async.GoSafe(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		n.pushAll(ctx, ev)
	})
}

func (n *Notifier) pushAll(ctx context.Context, ev *RunEvent) {
	for i := range n.cfg.Ntfy {
		cfg := &n.cfg.Ntfy[i]
		if !wantEvent(cfg.Events, ev.Status) {
			continue
		}
		if err := n.sendNtfy(ctx, cfg, ev); err != nil {
			slog.Warn("notify ntfy failed", "error", redactError(err), "url", redactSecretURL(cfg.URL))
		}
	}
	for i := range n.cfg.Gotify {
		cfg := &n.cfg.Gotify[i]
		if !wantEvent(cfg.Events, ev.Status) {
			continue
		}
		if err := n.sendGotify(ctx, cfg, ev); err != nil {
			slog.Warn("notify gotify failed", "error", redactError(err), "url", redactSecretURL(cfg.URL))
		}
	}
	if n.cfg.Webhook != nil {
		if err := n.sendWebhook(ctx, n.cfg.Webhook, ev); err != nil {
			slog.Warn("notify webhook failed", "error", redactError(err), "url", redactSecretURL(n.cfg.Webhook.FailURL))
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
	// 局部变量别叫 url:它遮蔽了 net/url 包(importShadow)。
	endpoint := cfg.URL
	if cfg.Topic != "" {
		endpoint = strings.TrimRight(endpoint, "/") + "/" + cfg.Topic
	}
	body := formatBody(ev)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
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
	endpoint := strings.TrimRight(cfg.URL, "/") + "/message?token=" + cfg.Token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return n.do(req)
}

func (n *Notifier) do(req *http.Request) error {
	resp, err := n.client.Do(req)
	if err != nil {
		// *url.Error 的 Error() 含**完整 URL** ⇒ Gotify 的 token(在 query 里)
		// 会跟着错误进日志 ⇒ 必须脱敏。
		return redactError(err)
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

// sendWebhook 通用回调:成功/失败分路 POST JSON,可选 HMAC 签名。
func (n *Notifier) sendWebhook(ctx context.Context, cfg *WebhookConfig, ev *RunEvent) error {
	if cfg == nil {
		return nil
	}
	endpoint := cfg.FailURL
	if ev.Status != "failed" && cfg.SuccessURL != "" {
		endpoint = cfg.SuccessURL
	}
	if endpoint == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"event":       "sync_run",
		"status":      ev.Status,
		"task_key":    ev.TaskKey,
		"task_name":   ev.TaskName,
		"run_id":      ev.RunID,
		"trigger":     ev.Trigger,
		"error":       ev.Error,
		"error_type":  ev.ErrorType,
		"duration_ms": ev.Duration.Milliseconds(),
		"end_at":      ev.EndAt.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitFerry-Event", "sync_run")
	if cfg.SharedSecret != "" {
		req.Header.Set("X-GitFerry-Signature", signBody(cfg.SharedSecret, payload))
	}
	return n.do(req)
}

// signBody HMAC-SHA256,返回 "sha256=<hex>"。
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// ===== 日志脱敏 =====
//
// 通知目标的 URL **不能**原样进日志:
// ① Gotify 的 token 就在 query 里;
// ② healthchecks.io 的 ping UUID 在**路径**里(同样是秘密);
// ③ 任何 webhook 地址都可能是"带凭据的一次性地址"。
// 而 http.Client 返回的 *url.Error 的 Error() 里含**完整 URL** ⇒ 错误信息也要脱敏。

// isSecretParam 判断查询参数名是否是秘密。
func isSecretParam(k string) bool {
	switch strings.ToLower(k) {
	case "token", "access_token", "private_token", "password", "passwd", "secret", "api_key", "apikey", "key":
		return true
	}
	return false
}

// redactSecretURL 脱敏 URL:秘密类查询参数替换掉,路径整体掩掉(路径里可能有 UUID/一次性凭据)。
func redactSecretURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparsable-url>"
	}
	q := u.Query()
	for k := range q {
		if isSecretParam(k) {
			q.Set(k, "REDACTED")
		}
	}
	u.RawQuery = q.Encode()
	if u.Path != "" && u.Path != "/" {
		u.Path = "/***"
		u.RawPath = ""
	}
	return u.String()
}

// redactError 把错误里的 URL 脱敏(*url.Error 的消息含完整 URL)。
func redactError(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: redactSecretURL(ue.URL), Err: ue.Err}
	}
	return err
}
