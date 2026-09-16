package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// DangerTools 需要用户确认的工具集合(值恒为 true,只作集合用)。
var DangerTools = map[string]bool{
	"run_task":                 true,
	"test_repo_connection":     true,
	"test_platform_connection": true,
}

// confirmRequiredPayload 返回给模型的"需要确认"标记。
// 模型应据此告知用户等待确认;真正执行由确认请求直达工具完成。
func confirmRequiredPayload(toolName, token, argsJSON string) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(argsJSON), &args)
	b, _ := json.Marshal(map[string]any{
		"status":  "confirmation_required",
		"message": "该操作需要用户在界面上确认后才会执行,请告知用户已弹出确认请求,不要重复调用本工具",
		"tool":    toolName,
		"args":    args,
	})
	return string(b)
}

// dangerGuard 危险工具统一入口:未确认 → 登记待确认并返回标记;确认后
// (handler 已 consume 并 MarkConsumed)→ 放行执行真实动作。
func (r *Registry) dangerGuard(ctx context.Context, toolName, argsJSON string, exec func(ctx context.Context, argsJSON string) (string, error)) (string, error) {
	sc := ScopeFrom(ctx)
	if sc == nil {
		return `{"error":"内部错误: 会话上下文缺失,拒绝执行危险操作"}`, nil
	}
	if sc.HasConsumed(toolName, argsJSON) {
		return exec(ctx, argsJSON)
	}
	token, err := sc.SetPending(toolName, argsJSON)
	if err != nil {
		return fmt.Sprintf(`{"error":"登记确认请求失败: %s"}`, err), nil
	}
	// 把令牌带给装饰器/前端(经会话快照)
	sc.SetLastConfirm(toolName, token, argsJSON)
	return confirmRequiredPayload(toolName, token, argsJSON), nil
}

type runTaskInput struct {
	TaskKey string `json:"task_key" jsonschema:"要立即执行一次同步的任务 key"`
}

func (r *Registry) runTask(ctx context.Context, in runTaskInput) (string, error) {
	return r.dangerGuard(ctx, "run_task", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in runTaskInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return fmt.Sprintf(`{"status":"failed","error":"参数解析失败: %s"}`, err), nil
		}
		if err := r.svc.RunTaskWithTrigger(ctx, in.TaskKey, "ai_agent", nil); err != nil {
			return fmt.Sprintf(`{"status":"failed","error":"同步执行失败: %s"}`, err), nil
		}
		return `{"status":"ok","message":"同步已执行完成,详情见任务历史"}`, nil
	})
}

type connTestInput struct {
	Key string `json:"key" jsonschema:"仓库或平台 key"`
}

func (r *Registry) testRepoConnection(ctx context.Context, in connTestInput) (string, error) {
	return r.dangerGuard(ctx, "test_repo_connection", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in connTestInput
		_ = json.Unmarshal([]byte(args), &in)
		res, err := r.svc.TestConnection(ctx, in.Key)
		if err != nil {
			return fmt.Sprintf(`{"connected":false,"error":%q}`, err.Error()), nil
		}
		return marshalJSON(map[string]any{"connected": res.Success, "message": res.Message}), nil
	})
}

func (r *Registry) testPlatformConnection(ctx context.Context, in connTestInput) (string, error) {
	return r.dangerGuard(ctx, "test_platform_connection", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in connTestInput
		_ = json.Unmarshal([]byte(args), &in)
		res, err := r.svc.TestPlatformConnection(ctx, in.Key)
		if err != nil {
			return fmt.Sprintf(`{"connected":false,"error":%q}`, err.Error()), nil
		}
		return marshalJSON(map[string]any{"connected": res.Connected, "message": res.Message}), nil
	})
}
