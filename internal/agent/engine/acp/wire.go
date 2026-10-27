package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/oekit/pkg/errors"
	"io"
	"strings"
	"sync"
)

// protocolVersion ACP v1 的协议版本。
const protocolVersion = 1

// errConfig 配置错误。
type errConfig string

func (e errConfig) Error() string { return string(e) }

// rpcError JSON-RPC 错误对象。
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ACP 错误 %d: %s", e.Code, e.Message)
}

// rpcMessage JSON-RPC 2.0 信封。
//
// 同一结构覆盖三种消息:
//   - 请求:有 method + id
//   - 通知:有 method 无 id
//   - 响应:有 id + result/error
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// isResponse 响应报文。
func (m *rpcMessage) isResponse() bool { return m.Method == "" && len(m.ID) > 0 }

// isRequest 请求报文(agent → client,需要应答)。
func (m *rpcMessage) isRequest() bool { return m.Method != "" && len(m.ID) > 0 }

// isNotification 通知报文(agent → client,不应答)。
func (m *rpcMessage) isNotification() bool { return m.Method != "" && len(m.ID) == 0 }

// rpcID 把 id 归一化成字符串键(数字 1 与字符串 "1" 归一到同一个键)。
func rpcID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if s[0] == '"' {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
	}
	return s
}

// lineReader 逐行读 JSON 报文。
//
// **跳过非 JSON 行**:实测 acpx 会往 stdout 打横幅
// (如 "[acpx] session gftest (uuid) · /path · agent mcode"),
// 直接 Unmarshal 会失败。跳过的行计数上报,不做静默吞掉。
type lineReader struct {
	sc      *bufio.Scanner
	skipped int
}

func newLineReader(r io.Reader) *lineReader {
	sc := bufio.NewScanner(r)
	// ACP 报文可能带大 payload(rawOutput 里的文件内容),放大缓冲上限。
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return &lineReader{sc: sc}
}

// next 取下一条**有效** JSON-RPC 报文。
// 返回 (nil, nil) 表示流已结束。
func (r *lineReader) next() (*rpcMessage, error) {
	for r.sc.Scan() {
		line := strings.TrimSpace(r.sc.Text())
		if line == "" {
			continue
		}
		var m rpcMessage
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			r.skipped++
			continue
		}
		return &m, nil
	}
	if err := r.sc.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// client 是 JSON-RPC 2.0 over stdio 的最小客户端(原生 transport 用)。
type client struct {
	stdin   io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[string]chan *rpcMessage
	closed  chan struct{}
	// onMessage 处理 agent 发来的请求与通知。
	onMessage func(*rpcMessage)
	// stderr 收集 agent 的 stderr(报错时给用户看)。
	stderr *ringBuffer
}

// newClient 构造客户端。
func newClient(stdin io.WriteCloser, stderr *ringBuffer) *client {
	return &client{
		stdin:   stdin,
		pending: make(map[string]chan *rpcMessage),
		closed:  make(chan struct{}),
		stderr:  stderr,
	}
}

// write 写一条报文。
func (c *client) write(m *rpcMessage) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return errors.New("ACP 连接已关闭")
	default:
	}
	if _, err := c.stdin.Write(b); err != nil {
		return errors.Wrap(err, "写 ACP 报文失败")
	}
	return nil
}

// call 发请求并等响应。
func (c *client) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	ch := make(chan *rpcMessage, 1)
	key := fmt.Sprintf("%d", id)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
	}()

	req := &rpcMessage{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", id)), Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}
	if err := c.write(req); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return errors.New("ACP 连接已关闭")
	case msg := <-ch:
		if msg.Error != nil {
			return msg.Error
		}
		if result != nil && len(msg.Result) > 0 {
			if err := json.Unmarshal(msg.Result, result); err != nil {
				return errors.Wrap(err, fmt.Sprintf("解析 %s 响应失败", method))
			}
		}
		return nil
	}
}

// notify 发通知(不期待响应)。
func (c *client) notify(method string, params any) error {
	msg := &rpcMessage{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		msg.Params = b
	}
	return c.write(msg)
}

// respond 应答 agent 发来的请求。
func (c *client) respond(id json.RawMessage, result any) error {
	msg := &rpcMessage{JSONRPC: "2.0", ID: id}
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		msg.Result = b
	}
	return c.write(msg)
}

// respondError 应答一个错误。
func (c *client) respondError(id json.RawMessage, code int, message string) error {
	msg := &rpcMessage{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
	return c.write(msg)
}

// close 关闭连接(幂等)。
func (c *client) close() {
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return
	default:
		close(c.closed)
	}
	c.mu.Unlock()
	_ = c.stdin.Close()
}

// dispatch 处理一条收到的报文:响应派发给等待者,请求/通知交给 onMessage。
func (c *client) dispatch(m *rpcMessage) {
	if m.isResponse() {
		key := rpcID(m.ID)
		c.mu.Lock()
		ch, ok := c.pending[key]
		c.mu.Unlock()
		if ok {
			ch <- m
		}
		return
	}
	if c.onMessage != nil {
		c.onMessage(m)
	}
}

// ringBuffer 环形缓冲:收集 agent 的 stderr,报错时截取尾部。
type ringBuffer struct {
	mu   sync.Mutex
	buf  []byte
	max  int
	seen bool
}

func newRingBuffer(max int) *ringBuffer { return &ringBuffer{max: max} }

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = true
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.max {
		r.buf = r.buf[len(r.buf)-r.max:]
	}
	return len(p), nil
}

// String 取当前内容(尾部优先)。
func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
