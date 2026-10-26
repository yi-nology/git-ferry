// Package delegate 定义「把任务委派给外部 coding agent」的最小契约。
//
// **为什么单独成包**:tools(领域工具表)与 engine/acp(外部后端)都要用它,
// 但两者不能互相依赖 —— tools ← engine/acp 的方向不能反过来。
// 单独成叶子包,两边都只依赖它。
package delegate

import "context"

// Request 一次委派请求。
type Request struct {
	// Prompt 交给外部 agent 的任务描述(要自包含:外部 agent 看不到我们的上下文)。
	Prompt string
	// Agent 覆盖默认 agent 名(如 mcode/codex/claude);空=用配置里的。
	Agent string
	// Cwd 覆盖默认工作目录;空=用配置里的。
	Cwd string
}

// Runner 外部 coding agent 的委派执行器。
type Runner interface {
	// Delegate 跑一次任务,返回 agent 的最终文本。
	Delegate(ctx context.Context, req Request) (string, error)
}
