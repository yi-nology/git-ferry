package agent

import "github.com/yi-nology/git-ferry/internal/agent/engine"

// Event SSE 层与 agent 的中立事件。
//
// **别名而非新定义**:事件的事实源在 engine 包(后端无关),
// 这里保留别名让既有引用(`biz/handler` 的 SSE 适配等)一行不改。
type Event = engine.Event

// Usage token 用量统计(done 事件携带)。
type Usage = engine.Usage

// resultCap SSE tool_end 里工具结果的最大长度。
const resultCap = 600
