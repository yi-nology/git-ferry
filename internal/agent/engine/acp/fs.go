package acp

import (
	"encoding/json"
	"errors"
	"fmt"
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
// 工作目录内 —— 这是代码层能做的最小保证(更强的隔离要靠部署侧容器)。
type fsHandler struct {
	// root 允许访问的根目录(绝对路径)。
	root string
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
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	path, err := h.resolve(p.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // 路径已过沙箱校验
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
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
		return fmt.Errorf("参数解析失败: %w", err)
	}
	path, err := h.resolve(p.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(p.Content), 0o600); err != nil {
		return fmt.Errorf("写文件失败: %w", err)
	}
	return nil
}

// resolve 解析并校验路径:必须在沙箱根内。
func (h *fsHandler) resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path 不能为空")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.root, path)
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(h.root, clean)
	if err != nil {
		return "", fmt.Errorf("路径越界: %s", path)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界(只允许访问 %s 内的文件): %s", h.root, path)
	}
	return clean, nil
}
