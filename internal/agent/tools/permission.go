package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/oagentkit/guardx/permgate"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
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

// Mode 当前模式(进程级默认档)。
func (p *Policy) Mode() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mode
}

// ModeFor 取本次调用生效的权限模式。
//
// **会话优先**:会话里显式切过的档只影响这个会话 ——
// 之前档位是**进程级单例**,一个会话切成 readonly,
// 会连带把**其他所有会话**的写操作都拒掉(跨会话 DoS)。
// 会话没设过就用进程级默认档。
func (p *Policy) ModeFor(ctx context.Context) string {
	if ctx != nil {
		if sc := ScopeFrom(ctx); sc != nil {
			if m := sc.PermissionMode(); m != "" {
				return m
			}
		}
	}
	return p.Mode()
}

// Override 单独覆盖某工具动作。
func (p *Policy) Override(tool string, act Action) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.overrides[tool] = act
}

// Classify 判定工具动作。
//
// danger:该工具是否需要用户确认 —— **由调用方(Registry)传入**。
// Policy 不再自己查任何全局集合:「哪些工具危险」的事实源在注册处
// (repo.go 的 addDanger 标注),由 Registry 用 r.IsDanger() 取出来传进来。
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
func (p *Policy) Classify(ctx context.Context, tool string, danger bool) Action {
	p.mu.RLock()
	override, hasOverride := p.overrides[tool]
	p.mu.RUnlock()

	if hasOverride {
		return override
	}

	// **会话级模式优先**:会话里显式切过的档只影响这个会话。
	mode := p.ModeFor(ctx)
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
//
// 「该工具是否危险」由 **r.IsDanger()** 提供(事实源在注册处),Policy 自己不查集合。
func (r *Registry) guardByPolicy(ctx context.Context, toolName string) (Action, string) {
	danger := r.IsDanger(toolName)
	if r.policy == nil {
		if danger {
			return ActionConfirm, ""
		}
		return ActionReadOnly, ""
	}
	act := r.policy.Classify(ctx, toolName, danger)
	if act == ActionDenied {
		// **权限模式切换工具不受只读档拦截**:
		// 否则成了一条单行道(切到只读之后就再也切不回来)。
		// 它的安全性由 admin 角色闸门兜底(在工具内部判)。
		if toolName == ToolSetPermissionMode {
			return ActionConfirm, ""
		}
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

// setModeIn 切换**本会话**的工具权限模式。
//
// 三道闸门(之前一道都没有):
//   - **角色闸门**:只有 admin 能拨这个档(见 dangerGuard 的角色检查
//     与下面的 admin 检查)。档位拨错会拒掉写操作,这是治理动作。
//   - **确认**:走 dangerGuard —— 需要用户点头。
//   - **会话级**:档位落在会话上,不影响其他会话。
func (r *Registry) setModeIn(ctx context.Context, in setModeInput) (string, error) {
	// **参数与角色先判,再进确认链路**。
	//
	// 顺序很重要:否则 operator 会先看到一个确认弹窗,确认完才被告知
	// 「你没权限」—— 白白打扰用户,还让人以为闸门形同虚设。
	switch in.Mode {
	case ModeDefault, ModeReadOnly, ModeAudit:
	default:
		return errJSON("mode 必须是 default|readonly|audit", nil), nil
	}
	// **只有 admin 能拨这个档**。
	//
	// 之前这个工具既没有角色闸门也没有确认:模型自己就能把档位拨成
	// readonly,而档位对象是**进程级单例** —— 一个会话拨了,
	// 其他会话的写操作全被拒(跨会话影响)。
	role := authz.RoleFromContext(ctx)
	if !authz.IsAdmin(role) {
		return errJSON("forbidden: 切换工具权限模式需要 admin 角色(当前:"+
			string(role)+")", nil), nil
	}
	return r.dangerGuard(ctx, ToolSetPermissionMode, marshalJSON(in),
		func(ctx context.Context, args string) (string, error) {
			var in setModeInput
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return errJSON("参数解析失败", err), nil
			}
			if r.policy == nil {
				return errJSON("权限策略未启用", nil), nil
			}
			// **档位落在会话上,不是进程上**。
			sc := ScopeFrom(ctx)
			if sc == nil || !sc.SetPermissionMode(in.Mode) {
				return errJSON("权限模式是**会话级**设置,当前调用链路没有可写的会话上下文"+
					"(如无状态的 MCP 端点):请用 CLI 或内置 AI 助手切换", nil), nil
			}
			return marshalJSON(map[string]any{
				"status": "ok", "mode": in.Mode, "scope": "session",
			}), nil
		})
}

// json 占位
var _ = json.Marshal
