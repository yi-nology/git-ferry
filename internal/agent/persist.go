package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// resultBudget 超过该长度的工具输出落盘,只回预览。
const resultBudget = 24 * 1024

// persistDir 落盘目录(空=不持久化,只截断)。
var persistDir string

// SetPersistDir 设置工具大输出落盘目录。
func SetPersistDir(dir string) { persistDir = dir }

// persistResult 处理工具返回:超长则落盘并回预览信封(借鉴 zcode persisted-output)。
func persistResult(toolName, out string) string {
	if len(out) <= resultBudget {
		return out
	}
	if persistDir == "" {
		return out[:resultBudget] + "\n...(已截断)"
	}
	if err := os.MkdirAll(persistDir, 0o750); err != nil {
		return out[:resultBudget] + "\n...(已截断,持久化失败)"
	}
	name := fmt.Sprintf("%s-%d.json", toolName, time.Now().UnixNano())
	path := filepath.Join(persistDir, name)
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return out[:resultBudget] + "\n...(已截断,持久化失败)"
	}
	preview := out
	const previewChars = 2000
	if len(preview) > previewChars {
		preview = preview[:previewChars] + "\n..."
	}
	return fmt.Sprintf(
		"<persisted-output>\nOutput too large (%d bytes). Full output saved to: %s\n\nPreview (first %d):\n%s\n</persisted-output>",
		len(out), path, previewChars, preview)
}
