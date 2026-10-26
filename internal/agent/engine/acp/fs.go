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
// 工作目录内 —— 这是代码层能给到的最小保证(更强的隔离要靠部署侧容器)。
type fsHandler struct {
	// root 允许访问的根目录(**已解析符号链接**的绝对路径)。
	root string
}

// newFSHandler 构造 fs 处理器(解析根目录的符号链接)。
func newFSHandler(root string) (*fsHandler, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败: %w", err)
	}
	// **根目录本身也要解析**:macOS 上 /tmp 是 /private/tmp 的软链,
	// 不解析就会把工作区里的合法文件误判成越界。
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录符号链接失败: %w", err)
	}
	return &fsHandler{root: real}, nil
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
	data, err := os.ReadFile(path) //nolint:gosec // 路径已过沙箱校验(含符号链接解析)
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

// resolve 解析并校验路径:必须在沙箱根内(**含符号链接**)。
//
// **为什么要解析符号链接**:只做 filepath.Clean 的话,
// 工作区里一个指向 ~/.ssh/id_rsa 的软链,路径看起来仍在根下,
// 实际读的是根外的文件 ⇒ 沙箱形同虚设。
//
// 目标**不存在**时(写新文件、或读一个不存在的文件)⇒
// 解析**父目录**的符号链接后再拼文件名 ——
// 包含关系仍要在**解析后**判,否则根目录本身在软链下(macOS 的 /var)会误判。
func (h *fsHandler) resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path 不能为空")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.root, path)
	}
	clean := filepath.Clean(path)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if os.IsNotExist(err) {
			// 目标还不存在(写新文件):解析父目录,再拼文件名。
			parent, perr := filepath.EvalSymlinks(filepath.Dir(clean))
			if perr != nil {
				return "", fmt.Errorf("解析父目录失败: %w", perr)
			}
			joined := filepath.Join(parent, filepath.Base(clean))
			if err := h.checkInside(joined); err != nil {
				return "", err
			}
			return joined, nil
		}
		return "", fmt.Errorf("解析路径失败: %w", err)
	}
	// **在解析之后判包含**:macOS 上 /var → /private/var,
	// 先用未解析的路径比会误判合法路径越界。
	if err := h.checkInside(real); err != nil {
		return "", err
	}
	return real, nil
}

// checkInside 校验路径在沙箱根内。
func (h *fsHandler) checkInside(path string) error {
	rel, err := filepath.Rel(h.root, path)
	if err != nil {
		return fmt.Errorf("路径越界: %s", path)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("路径越界(只允许访问 %s 内的文件): %s", h.root, path)
	}
	return nil
}
