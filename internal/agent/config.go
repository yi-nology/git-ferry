// Package agent 提供 git-ferry 的 AI 助手能力(eino 编排层)。
// 本包不 import hertz:SSE/HTTP 适配在 biz/handler 层,便于内网壳复用。
package agent

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// APIKeyEnvVar 模型 API Key 的环境变量名。密钥不写入 config.yaml。
const APIKeyEnvVar = "GIT_SYNC_AI_API_KEY" //nolint:gosec // 环境变量名,非凭据字面量

// Config 对应 conf/config.yaml 的 ai 段(独立于 core 配置,core 的
// yaml.Unmarshal 忽略未知字段,新增段不影响引擎加载)。
type Config struct {
	Enabled            bool    `yaml:"enabled"`
	BaseURL            string  `yaml:"base_url"` // OpenAI 兼容端点,内网可指向 vLLM/Ollama
	Model              string  `yaml:"model"`
	Temperature        float64 `yaml:"temperature"`
	MaxTokens          int     `yaml:"max_tokens"`
	TimeoutSeconds     int     `yaml:"timeout_seconds"`
	MaxConcurrentChats int     `yaml:"max_concurrent_chats"`
	// MemoryPath AI 长期记忆文件(空=data/ai-memory.json)
	MemoryPath string `yaml:"memory_path"`
	// HistoryMax 会话保留的最大消息轮数,超出后压缩(0=默认 40)
	HistoryMax int `yaml:"history_max"`
	// MemoryManifest 运行时注入的记忆摘要(不进 yaml)
	MemoryManifest string `yaml:"-"`
}

// AI 配置的缺省值。
// 补默认的逻辑此前在四处各写一遍(配置校验 / 设置更新 / 设置转运行时 / runner 兜底),
// 改一个默认值要同步四处——收敛到这一处。
const (
	defaultTemperature        = 0.3
	defaultMaxTokens          = 2048
	defaultTimeoutSeconds     = 60
	defaultMaxConcurrentChats = 4
)

// applyDefaults 补齐零值字段(<=0 视为未设置)。
func (c *Config) applyDefaults() {
	if c.Temperature <= 0 {
		c.Temperature = defaultTemperature
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = defaultMaxTokens
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = defaultTimeoutSeconds
	}
	if c.MaxConcurrentChats <= 0 {
		c.MaxConcurrentChats = defaultMaxConcurrentChats
	}
}

// applyDefaults 补齐零值字段(<=0 视为未设置)。
// Settings 与 Config 是两份平行结构、这四个字段同名同义——默认值只在
// config.go 的常量里定义一次,两处只做同样的判零。
func (s *Settings) applyDefaults() {
	if s.Temperature <= 0 {
		s.Temperature = defaultTemperature
	}
	if s.MaxTokens <= 0 {
		s.MaxTokens = defaultMaxTokens
	}
	if s.TimeoutSeconds <= 0 {
		s.TimeoutSeconds = defaultTimeoutSeconds
	}
	if s.MaxConcurrentChats <= 0 {
		s.MaxConcurrentChats = defaultMaxConcurrentChats
	}
}

// ErrBusy 并发会话已达上限。
var ErrBusy = errors.New("ai_busy: 当前会话数已达上限,请稍后再试")

// LoadConfig 从 yaml 文件读取 ai 段;段缺失视为未启用,不报错。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 启动配置路径由部署方控制
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var overlay struct {
		AI *Config `yaml:"ai"`
	}
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if overlay.AI == nil {
		return &Config{}, nil
	}
	return overlay.AI, nil
}

// APIKeyFromEnv 读取模型 API Key。
func APIKeyFromEnv() string { return os.Getenv(APIKeyEnvVar) }

// Validate 校验启用条件并填充默认值。apiKey 为空时启用必须失败(fail-fast)。
func (c *Config) Validate(apiKey string) error {
	if !c.Enabled {
		return nil
	}
	if c.BaseURL == "" {
		return errors.New("ai.enabled 需要 ai.base_url(OpenAI 兼容端点)")
	}
	if c.Model == "" {
		return errors.New("ai.enabled 需要 ai.model")
	}
	if apiKey == "" {
		return fmt.Errorf("ai.enabled 需要环境变量 %s", APIKeyEnvVar)
	}
	c.applyDefaults()
	return nil
}
