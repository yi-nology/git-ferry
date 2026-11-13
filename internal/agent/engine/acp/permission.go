package acp

import (
	"fmt"
	"github.com/oekit/pkg/errors"
	"sync"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// permItem 一次挂起的授权请求。
type permItem struct {
	id        string
	sessionID string
	title     string
	kind      string
	options   []engine.PermissionOption
	// respond 把用户的选择回给 agent(闭包持有 JSON-RPC 请求 id)。
	respond func(optionID string) error
	ch      chan string
}

// permRegistry 挂起中的授权请求登记表。
//
// 状态在 **agent 进程侧**(JSON-RPC 请求还没应答),
// 与 git-ferry 自己的待确认令牌(confirm.go)是**两套不同的确认语义**,不要混用。
type permRegistry struct {
	mu    sync.Mutex
	items map[string]*permItem
}

func newPermRegistry() *permRegistry {
	return &permRegistry{items: make(map[string]*permItem)}
}

// add 登记一个挂起请求。
func (r *permRegistry) add(it *permItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[it.id] = it
}

// remove 移除。
func (r *permRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
}

// get 取。
func (r *permRegistry) get(id string) (*permItem, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	it, ok := r.items[id]
	return it, ok
}

// respond 用户(或自动决策)提交选择。
//
// optionID 为空 = **拒绝**(ACP 的 outcome=cancelled)—— 用户点了「拒绝」
// 而 agent 又没给 reject 选项时,也要能拒。
func (r *permRegistry) respond(requestID, optionID string) error {
	it, ok := r.get(requestID)
	if !ok {
		return fmt.Errorf("授权请求 %s 不存在或已过期", requestID)
	}
	if optionID == "" {
		r.remove(requestID)
		select {
		case it.ch <- "":
			return nil
		default:
			return errors.New("授权请求已被处理")
		}
	}
	// 校验 option 属于本次请求:不能回一个 agent 没给过的选项。
	valid := false
	for _, o := range it.options {
		if o.OptionID == optionID {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("授权选项不属于该请求")
	}
	r.remove(requestID)
	select {
	case it.ch <- optionID:
		return nil
	default:
		return errors.New("授权请求已被处理")
	}
}
