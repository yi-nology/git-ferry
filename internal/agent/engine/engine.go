// Package engine 定义 AI 后端(执行引擎)的中立契约。
//
// **为什么要有这一层**:内置 eino 助手与外部 ACP coding agent 是两种完全不同的
// 执行形态 —— 前者是「进程内调 OpenAI 兼容端点,工具在本进程执行」,后者是
// 「把子进程当 agent,工具在 agent 侧执行(经 MCP)」。二者唯一的共同点是:
// 一轮对话进,一串**与后端无关**的事件出。
//
// 所以接缝在**回合层**,不在模型层:ACP 根本没有 chat/completions,
// 硬塞进 model.BaseModel 只会得到一堆假动作。
//
// 本包只放**契约**(接口与中立类型),不含任何实现:
//   - Event/Usage 与 internal/agent 的事件同形 ⇒ SSE 契约零变化
//   - Engine 是后端接口
//   - Session 是后端可见的会话最小面
//
// 依赖方向:engine → tools(仅取 SessionScope 契约)。
// internal/agent(内置 eino 后端)与 internal/agent/engine/acp(外部后端)
// 都依赖 engine,彼此不依赖。
package engine

import (
	"context"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// 事件类型。
const (
	// EventStart 一轮开始(携带 session_id)。
	EventStart = "start"
	// EventDelta 增量文本。
	EventDelta = "delta"
	// EventThinking 思考过程(外部 agent 才有;前端不认识就忽略)。
	EventThinking = "thinking"
	// EventPlan 计划条目(外部 agent 才有)。
	EventPlan = "plan"
	// EventToolStart 工具调用开始。
	EventToolStart = "tool_start"
	// EventToolEnd 工具调用结束。
	EventToolEnd = "tool_end"
	// EventToolConfirm 危险工具需要用户确认(内置后端)。
	EventToolConfirm = "tool_confirm"
	// EventPermission 外部 agent 请求授权(ACP session/request_permission)。
	EventPermission = "permission"
	// EventAgentSession 外部 agent 的会话已建立(带它在 agent 侧的会话 ID)。
	//
	// 用途:外部 agent 与本服务**共用同一个会话库**(如 MiniMax Code 的
	// CLI 与桌面版),把这个 ID 透出来,用户就能在那个界面上找到这次会话
	// (查看记录、接着聊),而不必在对方的会话列表里大海捞针。
	EventAgentSession = "agent_session"
	// EventDone 一轮结束。
	EventDone = "done"
	// EventError 出错。
	EventError = "error"
)

// Event 中立事件:与后端无关,SSE 层的唯一契约。
//
// 字段与 internal/agent 原 Event 完全一致(前端零改动);
// 新增字段都是可选的,老前端不认识即忽略。
type Event struct {
	Type      string `json:"type"` // 见上面的事件类型常量
	Content   string `json:"content,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Args      string `json:"args,omitempty"`
	Result    string `json:"result,omitempty"`
	Token     string `json:"token,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Usage     *Usage `json:"usage,omitempty"`

	// AgentSessionID 外部 agent 侧的会话 ID(可用于在 agent 自己的界面里找到这次会话)。
	AgentSessionID string `json:"agent_session_id,omitempty"`
	// AgentLabel 外部 agent 的展示名(如 "MiniMax Code")。
	AgentLabel string `json:"agent_label,omitempty"`

	// PermissionID 授权请求 id(ACP permission 链路);非空时前端应回传 option_id。
	PermissionID string `json:"permission_id,omitempty"`
	// Options 授权选项(ACP permission 链路)。
	Options []PermissionOption `json:"options,omitempty"`
}

// PermissionOption 一个可选授权项。
//
// ACP 的 session/request_permission 允许 agent 给出多个选项
// (一次性允许 / 永久允许 / 拒绝),前端展示后由用户挑一个回传。
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"` // allow_once|allow_always|reject_once|reject_always
}

// Usage token 用量统计(done 事件携带)。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Turn 会话历史里的一条消息。
type Turn struct {
	Role    string // user | assistant | system
	Content string
}

// Session 后端可见的会话最小面。
//
// 内嵌 tools.SessionScope 是因为工具层(危险工具确认、权限档)要靠它取会话状态;
// 后端拿到的 ctx 已经带好 scope,这里只是让引擎能主动读写会话。
type Session interface {
	tools.SessionScope

	// ConsumePending 校验并消费一次待确认调用(返回原始参数 JSON)。
	//
	// 它是 git-ferry 确认链路的**专有**能力(令牌在 git-ferry 会话上),
	// 不属于 tools.SessionScope —— 外部后端没有这套语义。
	ConsumePending(toolName, token string) (string, error)

	// SessionID 会话标识(注意:不是字段,避免与既有 Session.ID 字段同名冲突)。
	SessionID() string
	// History 会话历史拷贝(供后端拼装上下文)。
	History() []Turn
	// Append 追加一条消息。
	Append(role, content string)
}

// Input 一轮对话的输入。
type Input struct {
	// Message 用户消息。
	Message string
	// Workspace 外部 agent 的工作目录(沙箱边界);内置后端忽略。
	Workspace string
}

// Engine 一个能跑一轮对话的后端。
//
// 契约(实现方必须遵守):
//   - ctx 取消 ⇒ 事件流必须终止(不得继续产事件、不得继续消耗外部资源);
//   - 返回的 cancel 幂等,可重复调用;
//   - 启动期致命错误以 Go error 返回;运行期错误以 error 事件呈现。
type Engine interface {
	// ID 后端标识:internal | acpx | acp。
	ID() string
	// Label 展示名(如 "MiniMax Code (ACP)"、"内置助手 (eino)")。
	Label() string
	// ModelName 展示用模型/agent 名。
	ModelName() string
	// Run 执行一轮对话,产出事件流。
	Run(ctx context.Context, sess Session, in Input) (<-chan Event, context.CancelFunc, error)
	// Close 释放后端资源(关子进程/清会话)。可返回 nil。
	Close() error
}

// Confirmer 支持「确认后直达执行」的后端(工具在本进程执行的内置后端才有)。
//
// 这与 ACP 的 permission 链路是**两套不同的确认语义**:
//   - Confirmer:git-ferry 自己的待确认令牌(危险工具)
//   - PermissionResponder:ACP 的 request id → option id
//
// 两者不可混用:前者的状态在 git-ferry 会话上,后者的状态在 agent 进程里。
type Confirmer interface {
	ExecuteConfirmed(ctx context.Context, sess Session, toolName, token string) (string, error)
}

// PermissionResponder 支持「把用户的授权决定回传给后端」的后端。
type PermissionResponder interface {
	// RespondPermission 把用户选定的授权项回传给后端。
	RespondPermission(ctx context.Context, sessionID, requestID, optionID string) error
}

// MemoryBinder 支持注入长期记忆(记忆进 system prompt)。
type MemoryBinder interface {
	SetMemory(mem tools.MemStore, manifest string)
}

// DelegateStatus 可查「外部 agent 委派任务」状态的后端。
//
// 委派是异步的(见 tools 包的 delegate.go):确认后立刻返回任务 ID,
// 结果稍后才有 ⇒ 前端需要一个轮询入口。
type DelegateStatus interface {
	// DelegateStatus 返回 (状态快照, 是否存在)。
	DelegateStatus(taskID string) (map[string]any, bool)
}

// PolicyBinder 支持注入工具权限策略。
type PolicyBinder interface {
	SetPolicy(p *tools.Policy)
}

// Readiness 可选:后端的**静态就位性**(快,不启动进程、不消耗额度)。
//
// 与「测试连接」分工:Readiness 只答「装没装」(状态接口用);
// 「会不会说 ACP」要真握手,由各后端自己的 Probe 回答。
type Readiness interface {
	Ready() (bool, string)
}

// DelegatorBinder 支持注入「外部 coding agent 委派执行器」的后端。
//
// 只有**内置工具型**后端用得上:委派是内置助手的工具能力
// (ai_code_delegate),外部后端自己就是 agent,不需要再派出去。
type DelegatorBinder interface {
	SetDelegator(d delegate.Runner)
}
