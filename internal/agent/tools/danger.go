package tools

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// ToolSetPermissionMode 切换工具权限模式的工具名。
const ToolSetPermissionMode = "set_permission_mode"

// dangerCatalog 工具目录的**规范视图**:危险工具名的唯一来源。
//
// 它不是另一份手写清单 —— 危险标注写在注册处(repo.go 的 addDanger),
// 这里只是把同一份事实**取出来**给没有 Registry 实例的调用方
// (例如 internal/agent 的 system prompt 模板)。
//
// 之所以需要它:Registry 是按 service 构造的(每个进程/每个 handler 一份),
// 而提示词是**包级变量**、在 init 阶段就要成形,那时还没有任何 Registry。
// 工具名与危险标注都是**纯静态**的(不依赖 service),故用一份
// 不接 service 的目录注册表来派生,与真正的 Registry 用同一段注册代码 ⇒ 不会漂移。
var dangerCatalog = newCatalogRegistry()

// newCatalogRegistry 构造一份不接 service 的目录注册表。
//
// 只需要工具名与危险标记,故 svc 传 nil:工具的元信息(名字/描述)与 service 无关,
// 只有真正**执行**工具时才用到 svc(见 dangerGuard / 各工具实现)。
func newCatalogRegistry() *Registry {
	r := &Registry{byName: map[string]tool.InvokableTool{}, danger: map[string]bool{}}
	r.registerAll()
	return r
}

// DangerToolNames 全部危险工具名(排序)。
//
// **派生结果**:事实源是注册处(repo.go 的 addDanger 标注)。
// 供拿不到 Registry 实例的地方使用(如 system prompt 模板)。
func DangerToolNames() []string { return dangerCatalog.DangerToolNames() }

// IsDangerTool 该工具是否需要用户确认(危险工具)。
//
// **派生结果**:事实源是注册处(repo.go 的 addDanger 标注)。
func IsDangerTool(name string) bool { return dangerCatalog.IsDanger(name) }

// confirmRequiredPayload 返回给模型的"需要确认"标记。
// 模型应据此告知用户等待确认;真正执行由确认请求直达工具完成。
func confirmRequiredPayload(toolName, token, argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = map[string]any{"raw": argsJSON}
	}
	return marshalJSON(map[string]any{
		"status":  "confirmation_required",
		"message": "该操作需要用户在界面上确认后才会执行,请告知用户已弹出确认请求,不要重复调用本工具",
		"tool":    toolName,
		"token":   token,
		"args":    args,
	})
}

// dangerGuard 危险工具统一入口:未确认 → 登记待确认并返回标记;确认后
// (handler 已 consume 并 MarkConsumed)→ 放行执行真实动作。
func (r *Registry) dangerGuard(ctx context.Context, toolName, argsJSON string, exec func(ctx context.Context, argsJSON string) (string, error)) (string, error) {
	// **角色闸门**:危险工具要写角色。
	//
	// 之前这些工具只是「因为 MCP 路径不注入会话 scope」才被挡住的 ——
	// 那是偶然,不是设计:一旦有人注入 scope、或新增一个没套本函数的写工具,
	// 天窗立刻成立(只读用户就能触发推送/写裁决)。
	role := authz.RoleFromContext(ctx)
	if !authz.CanWrite(role) {
		return marshalJSON(map[string]any{
			"status": "failed",
			"error":  "forbidden: 角色 " + string(role) + " 不能执行 " + toolName,
		}), nil
	}
	if act, denied := r.guardByPolicy(ctx, toolName); act == ActionDenied {
		return denied, nil
	}
	sc := ScopeFrom(ctx)
	if sc == nil {
		// 这条链路没有会话上下文 → 没法走「登记待确认 → 用户确认 → 执行」。
		// 现在的入口:CLI 与 AI 助手有会话 ✓;MCP 端点(无状态)没有 ✗。
		//
		// 错误信息要说清「怎么办」,而不是抛一个内部错误:
		// 操作者看到「会话上下文缺失」只会以为程序坏了。
		return marshalJSON(map[string]any{
			"status": "failed",
			"error": "该操作需要用户确认,但当前调用链路没有会话上下文(如 MCP 端点):" +
				"请改用 CLI 或 HTTP 接口执行(那里有完整的确认流程)",
		}), nil
	}
	if sc.ConsumeConsumed(toolName, argsJSON) {
		return exec(ctx, argsJSON)
	}
	token, err := sc.SetPending(toolName, argsJSON)
	if err != nil {
		return errJSON("登记确认请求失败", err), nil
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
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		if err := r.svc.RunTaskWithTrigger(ctx, in.TaskKey, "ai_agent", nil); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "同步执行失败: " + err.Error()}), nil
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
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"connected": false, "error": "参数解析失败: " + err.Error()}), nil
		}
		res, err := r.svc.TestConnection(ctx, in.Key)
		if err != nil {
			return marshalJSON(map[string]any{"connected": false, "error": err.Error()}), nil
		}
		return marshalJSON(map[string]any{"connected": res.Success, "message": res.Message}), nil
	})
}

func (r *Registry) testPlatformConnection(ctx context.Context, in connTestInput) (string, error) {
	return r.dangerGuard(ctx, "test_platform_connection", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in connTestInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"connected": false, "error": "参数解析失败: " + err.Error()}), nil
		}
		res, err := r.svc.TestPlatformConnection(ctx, in.Key)
		if err != nil {
			return marshalJSON(map[string]any{"connected": false, "error": err.Error()}), nil
		}
		return marshalJSON(map[string]any{"connected": res.Connected, "message": res.Message}), nil
	})
}
