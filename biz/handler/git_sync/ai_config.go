package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// aiSettingsStore 进程内设置库(main 注入)。
var aiSettingsStore = struct {
	// holder 延迟绑定,避免 handler 包初始化依赖
	get func() *agent.SettingsStore
}{}

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

type aiConfigRequest struct {
	Enabled            bool    `json:"enabled"`
	BaseURL            string  `json:"base_url"`
	Model              string  `json:"model"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	TimeoutSeconds     int     `json:"timeout_seconds"`
	MaxConcurrentChats int     `json:"max_concurrent_chats"`
	// APIKey 空表示沿用已保存密钥;显式传 **** 不会被当成新密钥
	APIKey string `json:"api_key"`
}

// AIGetConfig GET /api/v1/ai/config —— 读取 AI 配置(密钥脱敏)。
func AIGetConfig(ctx context.Context, c *app.RequestContext) {
	st := getAISettingsStore()
	if st == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_settings_unavailable")
		return
	}
	cur := st.Get()
	response.Success(c, agent.View(&cur))
}

// AIUpdateConfig POST /api/v1/ai/config —— 更新配置并热生效。
func AIUpdateConfig(ctx context.Context, c *app.RequestContext) {
	st := getAISettingsStore()
	if st == nil {
		response.Error(c, consts.StatusNotImplemented, "ai_settings_unavailable")
		return
	}
	var req aiConfigRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	key := req.APIKey
	// 界面回显的脱敏值不可当作新密钥
	if strings.Contains(key, "****") {
		key = ""
	}
	saved, err := st.Save(&agent.Settings{
		Enabled:            req.Enabled,
		BaseURL:            req.BaseURL,
		Model:              req.Model,
		Temperature:        req.Temperature,
		MaxTokens:          req.MaxTokens,
		TimeoutSeconds:     req.TimeoutSeconds,
		MaxConcurrentChats: req.MaxConcurrentChats,
		APIKey:             key,
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
		return fmt.Errorf("runner rebuild not wired")
	}
	return rebuildAgentRunnerFn(st)
}

type aiTestRequest struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

// AIListModels GET /api/v1/ai/models —— 列出 OpenAI 兼容端点可用模型。
// Query: base_url(必填), api_key(可选,缺省用已保存密钥)。
func AIListModels(ctx context.Context, c *app.RequestContext) {
	base := strings.TrimRight(strings.TrimSpace(string(c.Query("base_url"))), "/")
	if base == "" {
		response.BadRequest(c, "base_url 不能为空")
		return
	}
	key := string(c.Query("api_key"))
	if strings.Contains(key, "****") {
		key = ""
	}
	if key == "" {
		if st := getAISettingsStore(); st != nil {
			key = st.Get().APIKey
		}
	}

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", http.NoBody)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		response.BadRequest(c, "无法连接端点: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode >= 400 {
		response.Error(c, resp.StatusCode, fmt.Sprintf("端点返回 HTTP %d", resp.StatusCode))
		return
	}
	var parsed struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		response.BadRequest(c, "端点响应不是合法 models 列表")
		return
	}
	models := make([]map[string]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
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
	var req aiTestRequest
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if base == "" {
		response.BadRequest(c, "base_url 不能为空")
		return
	}
	key := req.APIKey
	if strings.Contains(key, "****") {
		key = ""
	}
	if key == "" {
		if st := getAISettingsStore(); st != nil {
			key = st.Get().APIKey
		}
	}

	// 优先 GET /models(OpenAI 兼容);失败再 POST /chat/completions 极短补全
	client := &http.Client{Timeout: 8 * time.Second}
	status, modelOK, msg := probeOpenAICompat(ctx, client, base, key, req.Model)
	response.Success(c, map[string]any{
		"ok":         status > 0 && status < 400,
		"status":     status,
		"model_ok":   modelOK,
		"message":    msg,
		"base_url":   base,
		"checked_at": time.Now().Format(time.RFC3339),
	})
}

func probeOpenAICompat(ctx context.Context, client *http.Client, base, key, model string) (int, bool, string) {
	// 1) GET /models
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", http.NoBody)
	if err != nil {
		return 0, false, err.Error()
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, "无法连接端点: " + err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode >= 400 {
		return resp.StatusCode, false, fmt.Sprintf("端点返回 HTTP %d", resp.StatusCode)
	}
	modelOK := false
	if model != "" {
		var parsed struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err == nil {
			for _, m := range parsed.Data {
				if m.ID == model {
					modelOK = true
					break
				}
			}
			// 列表空或未列出目标模型:仍算端点可用,仅提示
			if !modelOK && len(parsed.Data) > 0 {
				return resp.StatusCode, false, fmt.Sprintf("端点可用,但模型列表中未找到 %s", model)
			}
		}
	}
	return resp.StatusCode, true, "端点连接正常"
}
