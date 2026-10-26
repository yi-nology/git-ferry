package acp

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// sessionUpdate session/update 通知里的 update 对象。
//
// 字段形状取自**实测**(mcode acp 0.6.2 / acpx 0.21.0):
//
//	{"sessionUpdate":"agent_message_chunk","messageId":"...","content":{"type":"text","text":"..."}}
//	{"sessionUpdate":"tool_call","toolCallId":"call_x","title":"read","name":"read","kind":"read"}
//	{"sessionUpdate":"tool_call_update","toolCallId":"call_x","status":"in_progress","rawInput":{...}}
//	{"sessionUpdate":"tool_call_update","toolCallId":"call_x","status":"completed","rawOutput":{...}}
//
// 注意:**tool_call_update 可能不带 name/title**(实测 mcode 只带 toolCallId),
// 所以工具名要在 tool_call 时记住 —— 见 updateMapper。
type sessionUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"`
	MessageID     string        `json:"messageId,omitempty"`
	Content       *contentBlock `json:"content,omitempty"`

	// 工具调用
	ToolCallID string          `json:"toolCallId,omitempty"`
	Title      string          `json:"title,omitempty"`
	Name       string          `json:"name,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Status     string          `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage `json:"rawOutput,omitempty"`
	Locations  []acpLocation   `json:"locations,omitempty"`

	// 计划
	Entries []planEntry `json:"entries,omitempty"`

	// 用量(agent 扩展,非 ACP 基线)
	Used int64 `json:"used,omitempty"`
	Size int64 `json:"size,omitempty"`
}

// acpLocation 工具涉及的文件位置。
type acpLocation struct {
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// planEntry 计划条目。
type planEntry struct {
	Content  string `json:"content,omitempty"`
	Priority string `json:"priority,omitempty"`
	Status   string `json:"status,omitempty"`
}

// contentBlock ACP 内容块。
type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
	Resource *struct {
		URI  string `json:"uri,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"resource,omitempty"`
}

// text 取内容块的文本。
func (c *contentBlock) text() string {
	if c == nil {
		return ""
	}
	if c.Text != "" {
		return c.Text
	}
	if c.Resource != nil && c.Resource.Text != "" {
		return c.Resource.Text
	}
	if c.URI != "" {
		return c.URI
	}
	return ""
}

// updateMapper 把 session/update 序列映射成中性事件。
//
// 需要状态:工具名在 tool_call 时记住(tool_call_update 可能不带名字)。
type updateMapper struct {
	mu        sync.Mutex
	toolNames map[string]string // toolCallId → 工具名
	toolKinds map[string]string // toolCallId → 工具种类
}

func newUpdateMapper() *updateMapper {
	return &updateMapper{
		toolNames: make(map[string]string),
		toolKinds: make(map[string]string),
	}
}

// map 把一条 update 映射成事件。
//
// 返回 nil 表示这条更新对前端无意义(忽略)。
// 事件语义与内置后端对齐:tool_start / tool_end / delta / thinking / plan。
func (m *updateMapper) mapUpdate(u *sessionUpdate) *engine.Event {
	if u == nil {
		return nil
	}
	switch u.SessionUpdate {
	case "agent_message_chunk":
		t := u.Content.text()
		if t == "" {
			return nil
		}
		return &engine.Event{Type: engine.EventDelta, Content: t}

	case "agent_thought_chunk":
		t := u.Content.text()
		if t == "" {
			return nil
		}
		return &engine.Event{Type: engine.EventThinking, Content: t}

	case "user_message_chunk":
		// 用户消息回显:前端已经自己显示过了,忽略。
		return nil

	case "tool_call":
		name := u.Name
		if name == "" {
			name = u.Title
		}
		if u.ToolCallID != "" {
			m.mu.Lock()
			m.toolNames[u.ToolCallID] = name
			m.toolKinds[u.ToolCallID] = u.Kind
			m.mu.Unlock()
		}
		return &engine.Event{
			Type: engine.EventToolStart,
			Tool: name,
			Args: string(u.RawInput),
		}

	case "tool_call_update":
		// 只有终态才产出 tool_end;中间态(in_progress 等)由前端那行"调用中…"顶着。
		switch u.Status {
		case "completed", "failed":
		default:
			return nil
		}
		return &engine.Event{
			Type:   engine.EventToolEnd,
			Tool:   m.toolName(u.ToolCallID, u),
			Result: toolOutputText(u.RawOutput, u.Status),
		}

	case "plan":
		text := planText(u.Entries)
		if text == "" {
			return nil
		}
		return &engine.Event{Type: engine.EventPlan, Content: text}

	default:
		// available_commands_update / session_info_update / usage_update /
		// current_mode_update 等:对当前 UI 无意义,忽略。
		return nil
	}
}

// toolName 取工具名:优先用本次更新带的,否则回查记住的。
func (m *updateMapper) toolName(callID string, u *sessionUpdate) string {
	if u != nil {
		if u.Name != "" {
			return u.Name
		}
		if u.Title != "" {
			return u.Title
		}
	}
	if callID == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolNames[callID]
}

// toolOutputText 从 rawOutput 提取可读文本。
//
// 实测形状:{"content":[{"type":"text","text":"..."}],"details":{...}}
func toolOutputText(raw json.RawMessage, status string) string {
	if len(raw) == 0 {
		if status == "failed" {
			return "执行失败"
		}
		return ""
	}
	var out struct {
		Content []contentBlock `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err == nil {
		var parts []string
		for i := range out.Content {
			if t := out.Content[i].text(); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	// 没有 content 块时回退成原始 JSON(截断由调用方负责)。
	s := string(raw)
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	return s
}

// planText 把计划条目拼成文本。
func planText(entries []planEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if e.Content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Content)
	}
	return b.String()
}
