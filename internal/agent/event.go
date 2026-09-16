package agent

// Event SSE 层与 agent 的中立事件(decorator/Runner 产出,handler 消费)。
type Event struct {
	Type      string `json:"type"` // start|delta|tool_start|tool_end|tool_confirm|done|error
	Content   string `json:"content,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Args      string `json:"args,omitempty"`
	Result    string `json:"result,omitempty"`
	Token     string `json:"token,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Usage     *Usage `json:"usage,omitempty"`
}

// Usage token 用量统计(done 事件携带)。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// resultCap SSE tool_end 里工具结果的最大长度。
const resultCap = 600

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
