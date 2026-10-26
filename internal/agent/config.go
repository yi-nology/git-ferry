// Package agent 提供 git-ferry 的 AI 助手能力。
//
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

// 执行后端标识。
//
// 后端决定「谁来跑这一轮对话」:
//   - internal:内置 eino 助手,进程内调 OpenAI 兼容端点,工具在本进程执行;
//   - acpx:外部成熟 coding 工具(经 acpx CLI 驱动);
//   - stdio:外部成熟 coding 工具(原生 ACP,自己当 client)。
//
// 外部后端下,工具在 **agent 侧**执行(经 git-ferry 自己的 /mcp)。
const (
	// BackendInternal 内置 eino 助手。
	BackendInternal = "internal"
	// BackendACPX 外部 coding agent(acpx CLI)。
	BackendACPX = "acpx"
	// BackendStdio 外部 coding agent(原生 ACP stdio)。
	BackendStdio = "stdio"
)

// Config 对应 conf/config.yaml 的 ai 段(独立于 core 配置,core 的
// yaml.Unmarshal 忽略未知字段,新增段不影响引擎加载)。
type Config struct {
	Enabled            bool    `yaml:"enabled"`
	Backend            string  `yaml:"backend"`  // internal|acpx|stdio;留空=internal
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

	// External 外部 coding agent 配置(backend=acpx|stdio 时生效)。
	External ExternalConfig `yaml:"external"`
}

// ExternalConfig 外部 coding agent 配置。
type ExternalConfig struct {
	// Agent acpx 内置 agent 名(mcode / codex / claude / gemini / opencode …)。
	Agent string `yaml:"agent"`
	// Command 自定义命令(原生 ACP 用,如 "mcode acp")。
	Command string `yaml:"command"`
	// Args 追加参数。
	Args []string `yaml:"args"`
	// Cwd 工作目录(沙箱边界)。
	Cwd string `yaml:"cwd"`
	// TimeoutSeconds 单轮超时。
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// Permission 授权档:ask|approve_reads|approve_all|deny。
	Permission string `yaml:"permission"`
	// MCPEnabled 是否把 git-ferry 自己的 /mcp 挂给外部 agent。
	MCPEnabled bool `yaml:"mcp_enabled"`
	// MCPURL git-ferry 自己的 MCP 端点地址。
	MCPURL string `yaml:"mcp_url"`
	// MCPAPIKeyEnv 挂给外部 agent 的 MCP 专用 key 的环境变量名。
	//
	// **密钥只走环境变量**:不进 yaml、不进设置文件。
	// 该 key 应当是**只读角色** —— MCP 路径上危险工具不弹确认(见 internal/mcp/server.go),
	// 用 admin key 挂出去等于把「无需确认的写操作」交出去。
	MCPAPIKeyEnv string `yaml:"mcp_api_key_env"`

	// MCPServers 额外挂给外部 agent 的 MCP 服务器(可多个)。
	//
	// 与上面三个字段的关系:MCPEnabled/MCPURL/MCPAPIKeyEnv 说的是
	// (把**本服务自己的** /mcp 挂出去);这里是**别的** MCP 服务器。
	// 最终挂载清单 = (MCPEnabled ? 本服务 /mcp : 无) + MCPServers。
	MCPServers []MCPServerConfig `yaml:"mcp_servers"`
}

// MCPServerConfig 一个额外的 MCP 服务器配置。
//
// **密钥不进配置**:只填环境变量名(api_key_env),构建时取值。
//
// yaml 与设置文件共用同一个结构体(双 tag)—— 两套平行结构迟早会漂。
type MCPServerConfig struct {
	// Name 人类可读标识(必填)。
	Name string `yaml:"name" json:"name"`
	// Type 传输类型:http | sse | stdio。留空=stdio。
	Type string `yaml:"type" json:"type"`
	// URL http/sse 传输的地址。
	URL string `yaml:"url" json:"url"`
	// APIKeyEnv 该服务器密钥的环境变量名(留空=不带认证头)。
	//
	// 环境变量里放的是**完整的头值**(如 "Bearer xxxxx"),不做任何拼接。
	APIKeyEnv string `yaml:"api_key_env" json:"api_key_env"`
	// APIKeyHeader 认证头名(留空=Authorization)。
	APIKeyHeader string `yaml:"api_key_header" json:"api_key_header"`
	// Command/Args/Env stdio 传输的可执行文件、参数与环境变量。
	Command string      `yaml:"command" json:"command"`
	Args    []string    `yaml:"args" json:"args"`
	Env     []EnvConfig `yaml:"env" json:"env"`
}

// EnvConfig 环境变量。
type EnvConfig struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}

// AI 配置的缺省值。
// 补默认的逻辑此前在四处各写一遍(配置校验 / 设置更新 / 设置转运行时 / runner 兜底),
// 改一个默认值要同步四处——收敛到这一处。
const (
	defaultTemperature        = 0.3
	defaultMaxTokens          = 2048
	defaultTimeoutSeconds     = 60
	defaultMaxConcurrentChats = 4
	defaultBackend            = BackendInternal
)

// applyDefaults 补齐零值字段(<= 0 视为未设置)。
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
	if c.Backend == "" {
		c.Backend = defaultBackend
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
	if s.Backend == "" {
		s.Backend = defaultBackend
	}
}

// NormalizePermission 归一化授权档(空 = 默认 ask)。
//
// 比较「有没有变更」与校验时都必须先过它:空串与默认值要等价,
// 否则「没填」会被当成「改了值」。
func NormalizePermission(p string) string {
	if p == "" {
		return "ask"
	}
	return p
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
//
// **按后端分支校验**:选了哪个后端,就按那个后端的要求校验。
// 这不是「给调用方兜底默认值」—— 后端是显式选择,不是可推断的缺省。
func (c *Config) Validate(apiKey string) error {
	if !c.Enabled {
		return nil
	}
	switch c.Backend {
	case "", BackendInternal:
		c.Backend = BackendInternal
		if c.BaseURL == "" {
			return errors.New("ai.backend=internal 需要 ai.base_url(OpenAI 兼容端点)")
		}
		if c.Model == "" {
			return errors.New("ai.backend=internal 需要 ai.model")
		}
		if apiKey == "" {
			return fmt.Errorf("ai.backend=internal 需要环境变量 %s", APIKeyEnvVar)
		}
	case BackendACPX:
		if c.External.Agent == "" && c.External.Command == "" {
			return errors.New("ai.backend=acpx 需要 ai.external.agent(如 mcode)或 ai.external.command")
		}
		if c.External.TimeoutSeconds <= 0 {
			c.External.TimeoutSeconds = 600
		}
		if c.External.Permission == "" {
			c.External.Permission = "ask"
		}
	case BackendStdio:
		if c.External.Command == "" {
			return errors.New("ai.backend=stdio 需要 ai.external.command(如 \"mcode acp\")")
		}
		if c.External.TimeoutSeconds <= 0 {
			c.External.TimeoutSeconds = 600
		}
		if c.External.Permission == "" {
			c.External.Permission = "ask"
		}
	default:
		return fmt.Errorf("未知 ai.backend: %s(可选 internal|acpx|stdio)", c.Backend)
	}
	c.applyDefaults()
	return nil
}
