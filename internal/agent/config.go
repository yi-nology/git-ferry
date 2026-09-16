// Package agent 提供 git-sync-service 的 AI 助手能力(eino 编排层)。
// 本包不 import hertz:SSE/HTTP 适配在 biz/handler 层,便于内网壳复用。
package agent

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// APIKeyEnvVar 模型 API Key 的环境变量名。密钥不写入 config.yaml。
const APIKeyEnvVar = "GIT_SYNC_AI_API_KEY"

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
	if c.Temperature <= 0 {
		c.Temperature = 0.3
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 2048
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 60
	}
	if c.MaxConcurrentChats <= 0 {
		c.MaxConcurrentChats = 4
	}
	return nil
}
