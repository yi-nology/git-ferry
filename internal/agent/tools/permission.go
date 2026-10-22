package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/oagentkit/guardx/permgate"
)

// Action 工具动作分级(借鉴 zcode permission capability groups)。
type Action string

const (
	ActionReadOnly Action = "readonly" // 只读,直接执行
	ActionConfirm  Action = "confirm"  // 危险,需用户确认
	ActionDenied   Action = "denied"   // 拒绝执行
)

// 权限模式常量。
const (
	// ModeDefault 危险工具走确认链路。
	ModeDefault = "default"
	// ModeReadOnly 一切写/危险工具一律拒绝。
	ModeReadOnly = "readonly"
	// ModeAudit 允许执行但标记审计(行为同 default)。
	ModeAudit = "audit"
)

// Policy 工具权限策略(可运行时切换,如只读模式)。
//
// 判定主体交给 oagentkit guardx/permgate:它把「要不要人确认」表达成
// 规则引擎(Mode + Capability + Ruleset),裁决结果带 RuleID/Reason,
// 且预留了按工具入参内容匹配、按会话授权等扩展位——
// 权限策略从此是数据而不是一段 switch,后续加配置化规则不必再重写判定层。
type Policy struct {
	mu        sync.RWMutex
	mode      string            // ModeDefault | ModeReadOnly | ModeAudit
	overrides map[string]Action // 工具名 → 动作(最高优先)
	gate      *permgate.Service
}

// NewPolicy 默认策略:危险工具 confirm,其余 readonly。
func NewPolicy() *Policy {
	return &Policy{
		mode:      ModeDefault,
		overrides: map[string]Action{},
		gate:      &permgate.Service{},
	}
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
//
// 映射关系(与迁移前的本地 switch 逐档等价):
//   - 危险工具 → Capability{AlwaysAsk} → permgate 裁决 Ask  → ActionConfirm
//   - 其余工具 → Capability{ReadOnly}  → permgate 裁决 Allow → ActionReadOnly
//   - 只读模式 → permgate ModeReadOnly → 非只读工具 Deny → ActionDenied
//
// 只留一处本地判定:显式 override 最高优先——permgate 的规则集排在只读档之下,
// 表达不了「把某个工具显式改成只读/放行」。
//
// 「只读模式下危险工具一律拒」这一条**已由上游原生支持**
// (oagentkit/guardx v0.14.0 起:checkAlwaysAsk 在会话授权之后、ask 之前插了
// 只读拦截段,checkStandard 把只读拦截插在 project ask 之前),本地不再兜底。
// 上游口径:会话授权在只读档仍放行(是用户已给出的决定,不是「问」)——
// 本仓从不调用 AllowForSession,故无行为差异。
func (p *Policy) Classify(tool string) Action {
	p.mu.RLock()
	override, hasOverride := p.overrides[tool]
	mode := p.mode
	p.mu.RUnlock()

	if hasOverride {
		return override
	}

	danger := DangerTools[tool]
	d := p.gate.Check(context.Background(), tool, nil, permgateMode(mode), toolCapability(danger))
	switch d.Behavior {
	case permgate.Allow:
		return ActionReadOnly
	case permgate.Ask:
		return ActionConfirm
	case permgate.Deny:
		return ActionDenied
	default:
		return ActionReadOnly
	}
}

// permgateMode 把本仓模式映射到 permgate 模式。
// 审计档行为同默认档(permgate 无对应档位)。
func permgateMode(mode string) permgate.Mode {
	if mode == ModeReadOnly {
		return permgate.ModeReadOnly
	}
	return permgate.ModeDefault
}

// toolCapability 工具能力自报:危险工具逐次确认,其余只读。
func toolCapability(danger bool) permgate.Capability {
	return permgate.Capability{ReadOnly: !danger, AlwaysAsk: danger}
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
	case ModeDefault, ModeReadOnly, ModeAudit:
		r.policy.SetMode(in.Mode)
		return marshalJSON(map[string]any{"status": "ok", "mode": in.Mode}), nil
	default:
		return errJSON("mode 必须是 default|readonly|audit", nil), nil
	}
}

// json 占位
var _ = json.Marshal
