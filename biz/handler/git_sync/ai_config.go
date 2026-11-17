package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/pkg/errors"
	"github.com/oekit/pkg/ptrx"

	"github.com/yi-nology/git-ferry/biz/model/ai"
	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/engine/acp"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// aiSettingsStore 进程内设置库(main 注入)。
var aiSettingsStore = struct {
	// holder 延迟绑定,避免 handler 包初始化依赖
	get func() *agent.SettingsStore
}{}

// aiYamlConfig yaml 里的 ai 段(main 注入)。
//
// **为什么要在 handler 侧合并**:设置文件只是**覆盖层**,
// 真正的生效值 = yaml 默认 + 设置文件覆盖。
// 只回设置文件的话,用 yaml 配置的部署在界面上看到的是**空表单**
// (用户得把配置全部重敲一遍才能保存)—— 那是"读到的不是真正在跑的那份"。
var aiYamlConfig = struct {
	mu  sync.RWMutex
	cfg *agent.Config
}{}

// SetAIYamlConfig 注入 yaml 段的 ai 配置。
func SetAIYamlConfig(cfg *agent.Config) {
	aiYamlConfig.mu.Lock()
	aiYamlConfig.cfg = cfg
	aiYamlConfig.mu.Unlock()
}

// effectiveAISettings 取**生效**的 AI 配置(yaml + 设置文件覆盖)。
func effectiveAISettings(st *agent.SettingsStore) agent.Settings {
	aiYamlConfig.mu.RLock()
	yamlCfg := aiYamlConfig.cfg
	aiYamlConfig.mu.RUnlock()
	cur := st.Get()
	if yamlCfg == nil {
		return cur
	}
	return agent.MergeSettings(yamlCfg, &cur)
}

// SetAISettingsStore 注入 AI 设置库。
func SetAISettingsStore(get func() *agent.SettingsStore) {
	aiSettingsStore.get = get
}

func getAISettingsStore() *agent.SettingsStore {
	if aiSettingsStore.get == nil {
		return nil
	}
	return aiSettingsStore.get()
}

// AIGetConfig GET /api/v1/ai/config —— 读取 AI 配置(密钥脱敏)。
func AIGetConfig(ctx context.Context, c *app.RequestContext) {
	st := getAISettingsStore()
	if st == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_settings_unavailable")
		return
	}
	// 回**生效**配置(不是设置文件原样)—— 见 effectiveAISettings 的说明。
	eff := effectiveAISettings(st)
	response.Success(c, agent.View(&eff))
}

// AIUpdateConfig POST /api/v1/ai/config —— 更新配置并热生效。
//
// **两道闸门**(AI 配置决定「用什么模型/什么端点/什么凭据/在本机跑什么 agent」):
//   - 写闸门:只读角色不能改 AI 配置;
//   - 治理闸门:**换后端**或**改外部 agent 授权档**仅 admin —— 外部 agent 是
//     跑在本机上的子进程,授权档决定它能做什么,这类决定不该由 operator 做出。
func AIUpdateConfig(ctx context.Context, c *app.RequestContext) {
	st := getAISettingsStore()
	if st == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_settings_unavailable")
		return
	}
	// 写闸门:只读角色不改 AI 配置。
	if !CanWriteRole(GetAuthRole(c)) {
		response.Biz(c, response.NewBizError(consts.StatusForbidden,
			"forbidden: 角色 "+string(GetAuthRole(c))+" 不能修改 AI 配置"))
		return
	}
	cur := effectiveAISettings(st)
	var req ai.AIConfigRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	key := ptrx.Deref(req.ApiKey, "")
	// 界面回显的脱敏值不可当作新密钥
	if strings.Contains(key, "****") {
		key = ""
	}
	backend := ptrx.Deref(req.Backend, "")
	if backend == "" {
		backend = agent.BackendInternal
	}
	// 未提交 external 时保留现有外部 agent 配置:部分客户端只改 model/base_url,
	// 零值覆盖会把已保存的 profile/授权档/命令全部抹掉。
	ext := cur.External
	if req.External != nil {
		ext = agent.ExternalSettings{
			Agent:          req.External.Agent,
			Command:        req.External.Command,
			Args:           req.External.Args,
			Cwd:            req.External.Cwd,
			TimeoutSeconds: int(req.External.TimeoutSeconds),
			Permission:     req.External.Permission,
			MCPEnabled:     req.External.McpEnabled,
			MCPURL:         req.External.McpUrl,
			MCPAPIKeyEnv:   req.External.McpApiKeyEnv,
			Current:        req.External.Current,
		}
		// 具名 profile 列表(可同时挂 Claude Code + Codex …)
		for _, a := range req.External.Agents {
			if a == nil {
				continue
			}
			profile := agent.ExternalAgentConfig{
				Name:           a.Name,
				Transport:      a.Transport,
				Agent:          a.Agent,
				Command:        a.Command,
				Args:           a.Args,
				Cwd:            a.Cwd,
				TimeoutSeconds: int(a.TimeoutSeconds),
				Permission:     a.Permission,
				MCPEnabled:     a.McpEnabled,
				MCPURL:         a.McpUrl,
				MCPAPIKeyEnv:   a.McpApiKeyEnv,
			}
			for _, m := range a.McpServers {
				if m == nil {
					continue
				}
				srv := agent.MCPServerConfig{
					Name:         m.Name,
					Type:         m.Type,
					URL:          m.URL,
					APIKeyEnv:    m.ApiKeyEnv,
					APIKeyHeader: m.ApiKeyHeader,
					Command:      m.Command,
					Args:         m.Args,
				}
				for _, e := range m.Env {
					if e == nil {
						continue
					}
					srv.Env = append(srv.Env, agent.EnvConfig{Name: e.Name, Value: e.Value})
				}
				profile.MCPServers = append(profile.MCPServers, srv)
			}
			ext.Agents = append(ext.Agents, profile)
		}
		for _, m := range req.External.McpServers {
			if m == nil {
				continue
			}
			srv := agent.MCPServerConfig{
				Name:         m.Name,
				Type:         m.Type,
				URL:          m.URL,
				APIKeyEnv:    m.ApiKeyEnv,
				APIKeyHeader: m.ApiKeyHeader,
				Command:      m.Command,
				Args:         m.Args,
			}
			for _, e := range m.Env {
				if e == nil {
					continue
				}
				srv.Env = append(srv.Env, agent.EnvConfig{Name: e.Name, Value: e.Value})
			}
			ext.MCPServers = append(ext.MCPServers, srv)
		}
	}
	// 治理闸门:换后端 / 改外部 agent 授权档 = 治理动作,仅 admin。
	//
	// 比较前**两边都要归一化**:存量后端可能是空串(等价 internal),
	// 不归一就会把「没改后端」误判成「换了后端」,把普通保存也卡在 admin 闸门上。
	//
	// 授权档要**连 profile 一起比**:profile 列表模式下改某个 profile 的授权档
	// 同样是治理动作,只比单字段会把它漏过去。
	curBackend := cur.Backend
	if curBackend == "" {
		curBackend = agent.BackendInternal
	}
	if backend != curBackend || !agent.SameExternalPermissions(&ext, &cur.External) {
		if GetAuthRole(c) != RoleAdmin {
			response.Biz(c, response.NewBizError(consts.StatusForbidden,
				"forbidden: 切换 AI 后端或修改外部 agent 授权档需要 admin 角色"))
			return
		}
		// 治理动作留审计:谁在什么时候把 AI 换成了什么、授权档改成了什么。
		recordAuditDetail(ctx, c, "ai.backend.change", "AIConfig", backend,
			"backend="+backend+",permission="+agent.NormalizePermission(ext.Permission),
			"from="+curBackend+",to="+backend)
	}
	saved, err := st.Save(&agent.Settings{
		Enabled:            req.Enabled,
		Backend:            backend,
		BaseURL:            req.BaseUrl,
		Model:              req.Model,
		Temperature:        req.Temperature,
		MaxTokens:          int(req.MaxTokens),
		TimeoutSeconds:     int(req.TimeoutSeconds),
		MaxConcurrentChats: int(req.MaxConcurrentChats),
		APIKey:             key,
		External:           ext,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 热重建 Runner(禁用则置 nil,端点回落 501)
	if err := rebuildAgentRunner(saved); err != nil {
		response.BadRequest(c, "配置已保存但启用失败: "+err.Error())
		return
	}
	response.Success(c, agent.View(saved))
}

// rebuildAgentRunner 按设置重建 Runner;由 main 注入实际构建逻辑。
var rebuildAgentRunnerFn func(*agent.Settings) error

// SetAIRebuildRunner 注入 Runner 重建函数。
func SetAIRebuildRunner(fn func(*agent.Settings) error) {
	rebuildAgentRunnerFn = fn
}

func rebuildAgentRunner(st *agent.Settings) error {
	if rebuildAgentRunnerFn == nil {
		return errors.New("runner rebuild not wired")
	}
	return rebuildAgentRunnerFn(st)
}

// aiListModelsReq AI 端点列表请求体。
type aiListModelsReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// AIListModels POST /api/v1/ai/models —— 列出 OpenAI 兼容端点可用模型。
// Body: base_url(必填), api_key(可选,缺省用已保存密钥)。
//
// **必须是 POST**:api_key 放 query 会进浏览器历史 / Referer / 反向代理 access log
// ⇒ 密钥泄漏(之前就是 GET+query)。
func AIListModels(ctx context.Context, c *app.RequestContext) {
	// 会把已保存密钥带给请求体指定的 base_url ⇒ 仅 admin。
	if !requireAdminRole(c) {
		return
	}
	var in aiListModelsReq
	if err := c.BindAndValidate(&in); err != nil {
		response.BadRequest(c, "请求体不合法: "+err.Error())
		return
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if base == "" {
		response.BadRequest(c, "base_url 不能为空")
		return
	}
	key := in.APIKey
	if strings.Contains(key, "****") {
		key = ""
	}
	if key == "" {
		key = savedAIKeyFor(base)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	entries, status, err := fetchOpenAIModels(ctx, client, base, key)
	if err != nil {
		switch {
		case status >= 400:
			response.Error(c, status, err.Error())
		case status == 0:
			response.BadRequest(c, "无法连接端点: "+err.Error())
		default:
			response.BadRequest(c, err.Error())
		}
		return
	}
	models := make([]map[string]string, 0, len(entries))
	for _, m := range entries {
		if m.ID == "" {
			continue
		}
		models = append(models, map[string]string{"id": m.ID, "owned_by": m.OwnedBy})
	}
	response.Success(c, map[string]any{
		"models":   models,
		"total":    len(models),
		"base_url": base,
	})
}

// AITestConfig POST /api/v1/ai/config/test —— 探测 OpenAI 兼容端点(不落盘)。
func AITestConfig(ctx context.Context, c *app.RequestContext) {
	// 外部 backend 会按请求体命令起进程 ⇒ 仅 admin。
	if !requireAdminRole(c) {
		return
	}
	var req ai.AITestRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	base := strings.TrimRight(strings.TrimSpace(req.BaseUrl), "/")
	if base == "" {
		response.BadRequest(c, "base_url 不能为空")
		return
	}
	key := ptrx.Deref(req.ApiKey, "")
	if strings.Contains(key, "****") {
		key = ""
	}
	if key == "" {
		key = savedAIKeyFor(base)
	}

	backend := ptrx.Deref(req.Backend, "")
	if backend == "" {
		backend = agent.BackendInternal
	}
	// **按后端分支探测**:外部 agent 根本没有 base_url/model,
	// 拿 OpenAI 兼容探测去问它是问错了对象。
	if backend != agent.BackendInternal {
		ok, msg := probeExternalAgent(ctx, backend, req.External)
		response.Success(c, map[string]any{
			"ok":         ok,
			"status":     200,
			"model_ok":   ok,
			"message":    msg,
			"base_url":   "",
			"backend":    backend,
			"checked_at": time.Now().Format(time.RFC3339),
		})
		return
	}
	// 优先 GET /models(OpenAI 兼容);失败再 POST /chat/completions 极短补全
	client := &http.Client{Timeout: 8 * time.Second}
	status, modelOK, msg := probeOpenAICompat(ctx, client, base, key, ptrx.Deref(req.Model, ""))
	response.Success(c, map[string]any{
		"ok":         status > 0 && status < 400,
		"status":     status,
		"model_ok":   modelOK,
		"message":    msg,
		"base_url":   base,
		"backend":    backend,
		"checked_at": time.Now().Format(time.RFC3339),
	})
}

// probeExternalAgent 探测外部 coding agent 后端是否就位。
//
// 探测的是**可执行性**(命令在不在、acpx 在不在),不是发真提示 ——
// 「测试连接」不该在服务端主机上跑一次真任务。
func probeExternalAgent(ctx context.Context, backend string, ext *ai.AIExternalConfig) (bool, string) {
	if ext == nil {
		return false, "缺少 external 配置"
	}
	cfg := acp.Config{
		Agent:      ext.Agent,
		Command:    ext.Command,
		Args:       ext.Args,
		Cwd:        ext.Cwd,
		Timeout:    time.Duration(ext.TimeoutSeconds) * time.Second,
		Permission: ext.Permission,
	}
	switch backend {
	case agent.BackendACPX:
		ver, err := acp.Probe(ctx, &cfg)
		if err != nil {
			return false, err.Error()
		}
		return true, "acpx 就绪(" + ver + ")"
	case agent.BackendStdio:
		// **真握手**:命令在 PATH 上 ≠ 它会说 ACP。
		// 起一次进程只发 initialize(不发 prompt,不消耗额度)。
		info, err := acp.ProbeStdio(ctx, &cfg)
		if err != nil {
			return false, err.Error()
		}
		return true, "ACP 握手成功:" + info
	default:
		return false, "未知后端: " + backend
	}
}

func probeOpenAICompat(ctx context.Context, client *http.Client, base, key, model string) (int, bool, string) {
	entries, status, err := fetchOpenAIModels(ctx, client, base, key)
	if err != nil {
		switch {
		case status >= 400:
			return status, false, err.Error()
		case status == 0:
			return 0, false, "无法连接端点: " + err.Error()
		default:
			// 端点可达但响应不是合法 models 列表:不能报 model_ok=true。
			return status, false, err.Error()
		}
	}
	modelOK := false
	if model != "" {
		for _, m := range entries {
			if m.ID == model {
				modelOK = true
				break
			}
		}
		// 列表空或未列出目标模型:仍算端点可用,仅提示
		if !modelOK && len(entries) > 0 {
			return status, false, fmt.Sprintf("端点可用,但模型列表中未找到 %s", model)
		}
	}
	return status, true, "端点连接正常"
}

// openAIModelEntry OpenAI 兼容 /models 响应项。
type openAIModelEntry struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}

// fetchOpenAIModels GET {base}/models —— 列模型与连通性探测共用同一实现,
// 统一 8s 客户端、256 KiB 限容与读错误处理。
func fetchOpenAIModels(ctx context.Context, client *http.Client, base, key string) ([]openAIModelEntry, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", http.NoBody)
	if err != nil {
		return nil, 0, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, resp.StatusCode, errors.Wrap(err, "读取端点响应失败")
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, errors.New(fmt.Sprintf("端点返回 HTTP %d", resp.StatusCode))
	}
	var parsed struct {
		Data []openAIModelEntry `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, resp.StatusCode, errors.New("端点响应不是合法 models 列表")
	}
	return parsed.Data, resp.StatusCode, nil
}

// savedAIKeyFor 取已保存的模型密钥——仅当探测地址与保存的 base_url 同源。
//
// 否则任意调用方都能把 "base_url = 自己的收集端" 传进来，让服务端把
// 已保存的模型密钥当 Authorization 头发出去（模型密钥外带）。
func savedAIKeyFor(base string) string {
	st := getAISettingsStore()
	if st == nil {
		return ""
	}
	s := st.Get()
	if s.APIKey == "" || !sameOrigin(base, s.BaseURL) {
		return ""
	}
	return s.APIKey
}

// sameOrigin 比较 scheme + host（忽略路径大小写差异）。
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(strings.TrimSpace(a))
	if err != nil || ua.Host == "" {
		return false
	}
	ub, err := url.Parse(strings.TrimSpace(b))
	if err != nil || ub.Host == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}
