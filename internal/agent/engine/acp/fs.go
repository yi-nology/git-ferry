package acp

import (
	"encoding/json"
	"github.com/oekit/pkg/errors"
	"os"
	"path/filepath"
	"strings"
)

// fsHandler 实现 **client 侧**的 fs/* 能力。
//
// ACP 规定:文件路径 **MUST 是绝对路径**,行号 1-based。
// 这里额外加一道**沙箱**:只允许访问工作目录内的文件。
//
// 为什么需要它:外部 agent 在 git-ferry 主机上跑,若 client 侧 fs 能力
// 是「无限制读宿主文件」,那它就等于把整机交出去了。沙箱把范围收在
// 工作目录内 —— 这是代码层能给到的最小保证(更强的隔离要靠部署侧容器)。
//
// 判定本身在 sandbox(与 terminal/* 共用同一份边界)。
type fsHandler struct {
	*sandbox
}

// newFSHandler 构造 fs 处理器(解析根目录的符号链接)。
func newFSHandler(root string) (*fsHandler, error) {
	sb, err := newSandbox(root)
	if err != nil {
		return nil, err
	}
	return &fsHandler{sandbox: sb}, nil
}

// readTextFile 处理 fs/read_text_file。
//
// 参数:{path, line?, limit?};返回 {content}。
func (h *fsHandler) readTextFile(raw json.RawMessage) (any, error) {
	var p struct {
		Path  string `json:"path"`
		Line  int    `json:"line"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errors.Wrap(err, "参数解析失败")
	}
	path, err := h.resolve(p.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // 路径已过沙箱校验(含符号链接解析)
	if err != nil {
		return nil, errors.Wrap(err, "读取文件失败")
	}
	content := string(data)
	if p.Line > 0 {
		lines := strings.Split(content, "\n")
		start := p.Line - 1 // ACP 行号 1-based
		if start >= len(lines) {
			return map[string]any{"content": ""}, nil
		}
		lines = lines[start:]
		if p.Limit > 0 && len(lines) > p.Limit {
			lines = lines[:p.Limit]
		}
		content = strings.Join(lines, "\n")
	}
	return map[string]any{"content": content}, nil
}

// writeTextFile 处理 fs/write_text_file。
func (h *fsHandler) writeTextFile(raw json.RawMessage) error {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return errors.Wrap(err, "参数解析失败")
	}
	path, err := h.resolve(p.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return errors.Wrap(err, "创建目录失败")
	}
	if err := os.WriteFile(path, []byte(p.Content), 0o600); err != nil {
		return errors.Wrap(err, "写文件失败")
	}
	return nil
}

// resolve 解析并校验路径:必须在沙箱根内(**含符号链接**)。
//
// 判定实现在 sandbox(与 terminal/* 共用)。
func (h *fsHandler) resolve(path string) (string, error) {
	if path == "" {
		return "", errEmptyPath
	}
	return h.sandbox.resolve(path)
}
