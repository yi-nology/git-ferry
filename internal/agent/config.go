// Package agent 提供 git-ferry 的 AI 助手能力。
//
// 本包不 import hertz:SSE/HTTP 适配在 biz/handler 层,便于内网壳复用。
package agent

import (
	"fmt"
	"github.com/oekit/pkg/errors"
	"os"
	"strings"

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
	Agent string `yaml:"agent" json:"agent"`
	// Command 自定义命令(原生 ACP 用,如 "mcode acp")。
	Command string `yaml:"command" json:"command"`
	// Args 追加参数。
	Args []string `yaml:"args" json:"args"`
	// Cwd 工作目录(沙箱边界)。
	Cwd string `yaml:"cwd" json:"cwd"`
	// TimeoutSeconds 单轮超时。
	TimeoutSeconds int `yaml:"timeout_seconds" json:"timeout_seconds"`
	// Permission 授权档:ask|approve_reads|approve_all|deny。
	Permission string `yaml:"permission" json:"permission"`
	// MCPEnabled 是否把 git-ferry 自己的 /mcp 挂给外部 agent。
	MCPEnabled bool `yaml:"mcp_enabled" json:"mcp_enabled"`
	// MCPURL git-ferry 自己的 MCP 端点地址。
	MCPURL string `yaml:"mcp_url" json:"mcp_url"`
	// MCPAPIKeyEnv 挂给外部 agent 的 MCP 专用 key 的环境变量名。
	//
	// **密钥只走环境变量**:不进 yaml、不进设置文件。
	// 该 key 应当是**只读角色** —— MCP 路径上危险工具不弹确认(见 internal/mcp/server.go),
	// 用 admin key 挂出去等于把「无需确认的写操作」交出去。
	MCPAPIKeyEnv string `yaml:"mcp_api_key_env" json:"mcp_api_key_env"`

	// MCPServers 额外挂给外部 agent 的 MCP 服务器(可多个)。
	//
	// 与上面三个字段的关系:MCPEnabled/MCPURL/MCPAPIKeyEnv 说的是
	// (把**本服务自己的** /mcp 挂出去);这里是**别的** MCP 服务器。
	// 最终挂载清单 = (MCPEnabled ? 本服务 /mcp : 无) + MCPServers。
	MCPServers []MCPServerConfig `yaml:"mcp_servers" json:"mcp_servers"`

	// Agents 具名的外部 agent profile 列表(可同时挂 Claude Code + Codex …)。
	//
	// **列表为空时**用上面那些单 profile 字段(保持既有语义,不改行为);
	// **列表非空时**以列表为准:`Current` 选中的 profile 驱动对话,
	// 委派工具的 agent 参数按名字选 profile。
	Agents []ExternalAgentConfig `yaml:"agents" json:"agents"`

	// Current 驱动对话的 profile 名(留空=列表第一个)。
	Current string `yaml:"current" json:"current"`
}

// ExternalAgentConfig 一个具名的外部 agent profile。
//
// 每个 profile 自带完整参数 ⇒ 切换 agent 只是换个名字,不用改一堆字段。
type ExternalAgentConfig struct {
	// Name profile 名(必填,也是委派工具里选它的关键字)。
	Name string `yaml:"name" json:"name"`
	// Transport 传输方式:acpx | stdio。留空=跟随 ai.backend。
	Transport string `yaml:"transport" json:"transport"`
	// Agent acpx 内置 agent 名(mcode/codex/claude…);stdio 形态留空。
	Agent string `yaml:"agent" json:"agent"`
	// Command 自定义命令(stdio 形态如 "mcode acp")。
	Command string `yaml:"command" json:"command"`
	// Args 追加参数。
	Args []string `yaml:"args" json:"args"`
	// Cwd 工作目录(沙箱边界)。
	Cwd string `yaml:"cwd" json:"cwd"`
	// TimeoutSeconds 单轮超时。
	TimeoutSeconds int `yaml:"timeout_seconds" json:"timeout_seconds"`
	// Permission 授权档:ask|approve_reads|approve_all|deny。
	Permission string `yaml:"permission" json:"permission"`
	// MCPEnabled 是否把 git-ferry 自己的 /mcp 挂给该 agent。
	MCPEnabled bool `yaml:"mcp_enabled" json:"mcp_enabled"`
	// MCPURL git-ferry 自己的 MCP 端点地址。
	MCPURL string `yaml:"mcp_url" json:"mcp_url"`
	// MCPAPIKeyEnv MCP 专用 key 的环境变量名(该 key 固定只读角色)。
	MCPAPIKeyEnv string `yaml:"mcp_api_key_env" json:"mcp_api_key_env"`
	// MCPServers 额外挂给该 agent 的 MCP 服务器。
	MCPServers []MCPServerConfig `yaml:"mcp_servers"`
}

// EffectiveExternal 返回本次**实际生效**的外部 agent profile 与传输方式。
//
// 所有校验与决策都走这里 —— 之前校验查的是旧的单字段(`external.command`),
// 而 profile 模式下命令在 profile 里 ⇒ 校验失败 ⇒ AI 根本没起来。
// 这类"新旧两套写法"的偏差,必须靠**单一入口**根治,不能靠逐处记得改。
//
// 传输方式:profile 自己写了 transport 就用它;否则跟随 ai.backend。
// **profile 的 transport 只在 ai.backend 已是外部后端时生效** ——
// ai.backend=internal 时 profile 列表只服务**委派**,不能把主后端顶掉。
func (c *Config) EffectiveExternal() (ExternalAgentConfig, string) {
	p := c.ResolveProfile(c.External.Current)
	transport := effectiveTransport(c.Backend, &p)
	return p, transport
}

// effectiveTransport 算出**主对话后端**实际生效的传输方式。
//
// 主后端不是外部后端 ⇒ 原样返回(internal),profile 的 transport 无权接管。
// profile 显式写了 transport ⇒ 以它为准;否则跟随 ai.backend。
func effectiveTransport(backend string, p *ExternalAgentConfig) string {
	if backend != BackendACPX && backend != BackendStdio {
		return backend
	}
	switch p.Transport {
	case BackendACPX, BackendStdio:
		return p.Transport
	}
	return backend
}

// ProfileTransportOf 算出某个 profile 独立使用时(委派)该走哪种传输。
//
// 优先级:
//  1. profile 自己写了 transport ⇒ 用它;
//  2. 否则有 command ⇒ stdio(命令就是一个 ACP agent 可执行文件);
//  3. 否则有 agent 名 ⇒ acpx(agent 名是 acpx 的内置 agent 名);
//  4. 否则跟随 ai.backend;ai.backend 也不是外部后端 ⇒ stdio。
//
// **与 effectiveTransport 的差别**:委派是独立于主后端的动作,
// 主后端是 internal 时 profile 列表照样能用 ⇒ 这里不能跟着主后端一起变 internal。
//
// 第 2/3 条的推断是必须的:老的单 profile 写法(只填 agent 或只填 command)
// 不带 transport,靠字段形状才知道该用哪个通道。
func ProfileTransportOf(backend string, p *ExternalAgentConfig) string {
	switch p.Transport {
	case BackendACPX, BackendStdio:
		return p.Transport
	}
	if p.Command != "" {
		return BackendStdio
	}
	if p.Agent != "" {
		return BackendACPX
	}
	switch backend {
	case BackendACPX, BackendStdio:
		return backend
	}
	return BackendStdio
}

// ResolveProfile 解析出要用的外部 agent profile。
//
// 规则:
//   - 列表非空 ⇒ 以列表为准:按 name 命中用它,否则用 Current 指定的,再否则用第一个;
//   - 列表为空 ⇒ 回退到单 profile 字段(旧行为,一个也不变)。
//
// 返回值里的 Transport 为空表示「跟随 ai.backend」。
func (c *Config) ResolveProfile(name string) ExternalAgentConfig {
	return resolveProfile(&c.External, c.Backend, name)
}

// effectiveProfile 解析出**生效**的外部 agent profile 与它的传输方式。
//
// 设置侧(保存校验)与配置侧(启动校验)共用 —— 两边判定不一致就是 bug
// (启动能起、保存不让存,或反过来)。
func effectiveProfile(ext *ExternalConfig, backend string) (ExternalAgentConfig, string) {
	p := resolveProfile(ext, backend, ext.Current)
	transport := effectiveTransport(backend, &p)
	return p, transport
}

// ValidateExternalProfile 完整校验一个外部 agent profile(参数 + acpx 的授权档)。
//
// **主对话与委派两种用法分别判**:
//   - 主对话:用 effectiveTransport 判(主后端是 internal 时它不参与);
//   - 委派:用 ProfileTransportOf 判(任何 profile 都可能被按名字选中)。
//
// acpx 是**非交互**通道(它的 stdin/stdout 被我们的调用占着)⇒ 弹不出授权卡,
// 授权档 ask 在主对话下名不副实 ⇒ 直接拒绝(不静默降级)。
// 委派侧不查 ask:一次性执行本来就把 ask 映射成 approve_reads。
func ValidateExternalProfile(p *ExternalAgentConfig, backend string) error {
	label := profileLabel(p)
	if err := CheckExternalProfileParams(p, effectiveTransport(backend, p), label); err != nil {
		return err
	}
	if err := CheckExternalProfileParams(p, ProfileTransportOf(backend, p), label); err != nil {
		return err
	}
	if ProfileUsesACPX(p, backend) && NormalizePermission(p.Permission) == PermissionAsk {
		return fmt.Errorf("%s 用的是 acpx(非交互),无法逐次弹授权卡(写操作会按拒绝处理):"+
			"授权档请选 approve_reads | approve_all | deny;要逐次确认请用 stdio 后端", label)
	}
	return nil
}

// CheckExternalProfileParams 按传输方式查 profile 的参数齐不齐。
func CheckExternalProfileParams(p *ExternalAgentConfig, transport, label string) error {
	switch transport {
	case BackendACPX:
		if p.Agent == "" && p.Command == "" {
			return fmt.Errorf("%s 名为空(填 agent 如 mcode,或填自定义 command)", label)
		}
	case BackendStdio:
		if p.Command == "" {
			return fmt.Errorf("%s 命令为空(如 \"mcode acp\")", label)
		}
	}
	return nil
}

// ProfileUsesACPX 该 profile 最终走的是不是 acpx 传输。
//
// profile 没写(或写了无效的)transport ⇒ 跟随 ai.backend。
func ProfileUsesACPX(p *ExternalAgentConfig, backend string) bool {
	switch p.Transport {
	case BackendACPX:
		return true
	case BackendStdio:
		return false
	}
	return backend == BackendACPX
}

// profileLabel 错误信息里对 profile 的称呼(具名的报名字,单 profile 的报「外部 agent」)。
func profileLabel(p *ExternalAgentConfig) string {
	if strings.TrimSpace(p.Name) != "" {
		return fmt.Sprintf("profile %q", p.Name)
	}
	return "外部 agent"
}

// resolveProfile 是包级实现 —— 配置侧(启动校验)与设置侧(保存校验)
// 必须用**同一套**解析规则,否则两边的判定会不一致(启动能起、保存不让存)。
func resolveProfile(ext *ExternalConfig, backend, name string) ExternalAgentConfig {
	if p, ok := resolveProfileByName(ext, name); ok {
		return p
	}
	if len(ext.Agents) == 0 {
		// 旧行为:单 profile 字段。
		return ExternalAgentConfig{
			Name:           ext.Agent,
			Transport:      backend,
			Agent:          ext.Agent,
			Command:        ext.Command,
			Args:           ext.Args,
			Cwd:            ext.Cwd,
			TimeoutSeconds: ext.TimeoutSeconds,
			Permission:     ext.Permission,
			MCPEnabled:     ext.MCPEnabled,
			MCPURL:         ext.MCPURL,
			MCPAPIKeyEnv:   ext.MCPAPIKeyEnv,
			MCPServers:     ext.MCPServers,
		}
	}
	for i := range ext.Agents {
		if ext.Agents[i].Name == ext.Current {
			return ext.Agents[i]
		}
	}
	return ext.Agents[0]
}

// ResolveProfileByName 按名字**精确**找 profile;找不到返回 (零值,false)。
//
// 委派工具要靠它区分「命中某个 profile」与「把名字当 acpx 内置 agent 名用」。
func (c *Config) ResolveProfileByName(name string) (ExternalAgentConfig, bool) {
	return resolveProfileByName(&c.External, name)
}

// resolveProfileByName 是包级实现(见 resolveProfile 的说明)。
func resolveProfileByName(ext *ExternalConfig, name string) (ExternalAgentConfig, bool) {
	if name == "" {
		return ExternalAgentConfig{}, false
	}
	for i := range ext.Agents {
		if ext.Agents[i].Name == name {
			return ext.Agents[i], true
		}
	}
	return ExternalAgentConfig{}, false
}

// ExternalProfileNames 列出已配置的 profile 名(界面展示用)。
func (c *Config) ExternalProfileNames() []string {
	out := make([]string, 0, len(c.External.Agents))
	for i := range c.External.Agents {
		out = append(out, c.External.Agents[i].Name)
	}
	return out
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
	// defaultExternalTimeoutSeconds 外部 coding agent 单轮默认超时(10 分钟)。
	//
	// 外部 agent 要起子进程、可能还要拉模型 ⇒ 比内置助手慢得多,
	// 用内置的 60s 会把正常任务判成超时。
	defaultExternalTimeoutSeconds = 600
)

// PermissionAsk 外部 agent 的授权档:每次写操作都问用户。
//
// 与 internal/agent/engine/acp.PermissionAsk 同值 —— 这里定义一份是为了
// 不让上层配置包反过来依赖后端实现包。
const PermissionAsk = "ask"

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
		return PermissionAsk
	}
	return p
}

// SameExternalPermissions 两份外部配置的授权档是否**完全一致**。
//
// **要逐个 profile 比**:只比单字段的话,profile 列表模式下改授权档
// 会被判成「没改」⇒ 绕过「改授权档需 admin」的治理闸门。
// 授权档决定外部 agent 能做什么(读 / 写 / 全放行)⇒ 这是安全边界,不能漏。
func SameExternalPermissions(a, b *ExternalConfig) bool {
	if NormalizePermission(a.Permission) != NormalizePermission(b.Permission) {
		return false
	}
	pa, pb := permissionByProfile(a), permissionByProfile(b)
	if len(pa) != len(pb) {
		return false
	}
	for name, perm := range pa {
		if pb[name] != perm {
			return false
		}
	}
	return true
}

// permissionByProfile 取出每个具名 profile 的归一化授权档。
func permissionByProfile(ext *ExternalConfig) map[string]string {
	out := make(map[string]string, len(ext.Agents))
	for i := range ext.Agents {
		out[ext.Agents[i].Name] = NormalizePermission(ext.Agents[i].Permission)
	}
	return out
}

// ErrBusy 并发会话已达上限。
var ErrBusy = errors.New("ai_busy: 当前会话数已达上限,请稍后再试")

// LoadConfig 从 yaml 文件读取 ai 段;段缺失视为未启用,不报错。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 启动配置路径由部署方控制
	if err != nil {
		return nil, errors.Wrap(err, "read config")
	}
	var overlay struct {
		AI *Config `yaml:"ai"`
	}
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return nil, errors.Wrap(err, "parse config")
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
//
// 外部后端的校验对象是**生效的那个 profile**(EffectiveExternal),
// 不是旧的单字段 —— profile 模式下命令在 profile 里,查单字段会误判成
// 「没配外部 agent」⇒ 启动直接失败 ⇒ AI 根本没起来。
func (c *Config) Validate(apiKey string) error {
	if !c.Enabled {
		return nil
	}
	c.applyDefaults()
	switch c.Backend {
	case BackendInternal:
		if c.BaseURL == "" {
			return errors.New("ai.backend=internal 需要 ai.base_url(OpenAI 兼容端点)")
		}
		if c.Model == "" {
			return errors.New("ai.backend=internal 需要 ai.model")
		}
		if apiKey == "" {
			return fmt.Errorf("ai.backend=internal 需要环境变量 %s", APIKeyEnvVar)
		}
	case BackendACPX, BackendStdio:
		c.applyExternalDefaults()
		// **每个 profile 都查**(不只生效的那个):坏 profile 在启动时就该报出来,
		// 而不是等哪天有人委派到它才炸。
		for i := range c.External.Agents {
			p := c.External.Agents[i]
			if strings.TrimSpace(p.Name) == "" {
				return errors.New("ai.external.agents[] 里的 profile 缺 name")
			}
			if err := CheckExternalProfileParams(&p, ProfileTransportOf(c.Backend, &p), profileLabel(&p)); err != nil {
				return errors.Wrap(err, fmt.Sprintf("ai.external.agents[%d]", i))
			}
		}
		p, transport := c.EffectiveExternal()
		switch transport {
		case BackendACPX:
			if p.Agent == "" && p.Command == "" {
				return errors.New("ai.backend=acpx 需要 ai.external.agent(如 mcode)或 ai.external.command")
			}
		case BackendStdio:
			if p.Command == "" {
				return errors.New("ai.backend=stdio 需要 ai.external.command(如 \"mcode acp\")")
			}
		default:
			return fmt.Errorf("未知外部传输方式: %s(可选 acpx|stdio)", transport)
		}
	default:
		return fmt.Errorf("未知 ai.backend: %s(可选 internal|acpx|stdio)", c.Backend)
	}
	return nil
}

// applyExternalDefaults 给外部 agent 配置补默认(单轮超时/授权档)。
//
// **两处都要补**:旧的单 profile 字段(列表为空时的回退源)与每个具名 profile。
// 只补一处 ⇒ 一边有默认一边没有 ⇒ 行为随写法漂移。
func (c *Config) applyExternalDefaults() {
	if c.External.TimeoutSeconds <= 0 {
		c.External.TimeoutSeconds = defaultExternalTimeoutSeconds
	}
	if c.External.Permission == "" {
		c.External.Permission = PermissionAsk
	}
	for i := range c.External.Agents {
		p := &c.External.Agents[i]
		if p.TimeoutSeconds <= 0 {
			p.TimeoutSeconds = defaultExternalTimeoutSeconds
		}
		if p.Permission == "" {
			p.Permission = PermissionAsk
		}
	}
}
