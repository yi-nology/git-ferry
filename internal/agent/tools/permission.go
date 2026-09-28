package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Action 工具动作分级(借鉴 zcode permission capability groups)。
type Action string

const (
	ActionReadOnly Action = "readonly" // 只读,直接执行
	ActionConfirm  Action = "confirm"  // 危险,需用户确认
	ActionDenied   Action = "denied"   // 拒绝执行
)

// Policy 工具权限策略(可运行时切换,如只读模式)。
type Policy struct {
	mu        sync.RWMutex
	mode      string            // "default" | "readonly" | "audit"
	overrides map[string]Action // 工具名 → 动作
}

// NewPolicy 默认策略:危险工具 confirm,其余 readonly。
func NewPolicy() *Policy {
	return &Policy{mode: "default", overrides: map[string]Action{}}
}

// SetMode 切换模式:
//   - default: 危险工具走确认链路
//   - readonly: 一切写/危险工具一律拒绝
//   - audit: 允许执行但标记审计(行为同 default)
func (p *Policy) SetMode(mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
}

// Mode 当前模式。
func (p *Policy) Mode() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mode
}

// Override 单独覆盖某工具动作。
func (p *Policy) Override(tool string, act Action) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.overrides[tool] = act
}

// Classify 判定工具动作。
func (p *Policy) Classify(tool string) Action {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if act, ok := p.overrides[tool]; ok {
		return act
	}
	danger := DangerTools[tool]
	switch p.mode {
	case "readonly":
		if danger {
			return ActionDenied
		}
		return ActionReadOnly
	default:
		if danger {
			return ActionConfirm
		}
		return ActionReadOnly
	}
}

// SetPolicy 注入策略到注册表。
func (r *Registry) SetPolicy(p *Policy) { r.policy = p }

// Policy 返回策略(可为 nil)。
func (r *Registry) Policy() *Policy { return r.policy }

// guardByPolicy 在 dangerGuard 之前套一层权限检查。
func (r *Registry) guardByPolicy(toolName string) (Action, string) {
	if r.policy == nil {
		if DangerTools[toolName] {
			return ActionConfirm, ""
		}
		return ActionReadOnly, ""
	}
	act := r.policy.Classify(toolName)
	if act == ActionDenied {
		return act, marshalJSON(map[string]any{
			"status":  "denied",
			"message": fmt.Sprintf("当前为只读模式,%s 被拒绝", toolName),
		})
	}
	return act, ""
}

// setModeInput 运行时切换权限模式。
type setModeInput struct {
	Mode string `json:"mode" jsonschema:"default|readonly|audit"`
}

func (r *Registry) setModeIn(_ context.Context, in setModeInput) (string, error) {
	if r.policy == nil {
		return errJSON("权限策略未启用", nil), nil
	}
	switch in.Mode {
	case "default", "readonly", "audit":
		r.policy.SetMode(in.Mode)
		return marshalJSON(map[string]any{"status": "ok", "mode": in.Mode}), nil
	default:
		return errJSON("mode 必须是 default|readonly|audit", nil), nil
	}
}

// json 占位
var _ = json.Marshal
