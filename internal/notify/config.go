// Package notify 提供同步结果的多通道通知与存活心跳。
//
// 通道模型(借鉴 gickup):
//   - ntfy/gotify:推送「运行完成」事件(成功/失败皆可配)
//   - heartbeat:  周期 GET 成功/失败 URL,对接 healthchecks.io 一类监视
package notify

// NtfyConfig ntfy.sh 推送配置。
type NtfyConfig struct {
	URL    string `yaml:"url"`
	Token  string `yaml:"token"`
	User   string `yaml:"user"`
	Pass   string `yaml:"password"`
	Topic  string `yaml:"topic"`
	Events []string `yaml:"events"` // 空=success+failed;可选 success/failed
}

// GotifyConfig Gotify 推送配置。
type GotifyConfig struct {
	URL    string   `yaml:"url"`
	Token  string   `yaml:"token"`
	Events []string `yaml:"events"`
}

// HeartbeatConfig 成功/失败分路心跳(healthchecks.io / deadmanssnitch)。
type HeartbeatConfig struct {
	SuccessURLs []string `yaml:"success_urls"`
	FailURLs    []string `yaml:"fail_urls"`
	// IntervalSeconds 周期;<=0 表示只随运行事件打点
	IntervalSeconds int `yaml:"interval_seconds"`
}

// Config 通知总配置(壳层 overlay,不进 core)。
type Config struct {
	Ntfy      []NtfyConfig      `yaml:"ntfy"`
	Gotify    []GotifyConfig    `yaml:"gotify"`
	Heartbeat *HeartbeatConfig  `yaml:"heartbeat"`
	// OnlyFail true 时仅通知失败运行,降低噪声
	OnlyFail bool `yaml:"only_fail"`
}

// Enabled 是否配置了任一通道。
func (c *Config) Enabled() bool {
	if c == nil {
		return false
	}
	return len(c.Ntfy) > 0 || len(c.Gotify) > 0 || c.Heartbeat != nil
}
