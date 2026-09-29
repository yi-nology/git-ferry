// Package textutil 收敛文本/路径安全化、CSV 与小工具,消除散落实现。
package textutil

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// SanitizePathToken 路径段安全化:去掉 / \ .. 与空格,防路径穿越。
func SanitizePathToken(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "..", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if s == "" {
		return "unnamed"
	}
	return s
}

// SanitizeFileToken 文件名安全化(路径分隔符/穿越防护)。
func SanitizeFileToken(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "..", "_")
	if s == "" {
		return "unnamed"
	}
	return s
}

// CSVEscape 转义 CSV 单元格(含逗号/引号/换行时加引号)。
func CSVEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// Truncate 按字节截断并追加省略号(保证 UTF-8 边界)。
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	// 回退到合法 UTF-8 边界
	for n > 0 && !utf8.ValidString(s[:n]) {
		n--
	}
	return s[:n] + "…"
}

// TruncateRunes 按字符(rune)截断。
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Itoa int 转字符串(统一入口)。
func Itoa(n int) string { return strconv.Itoa(n) }

// Itoa64 int64 转字符串。
func Itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// BoolFact bool → "true"/"false"(健康评分 facts 用)。
func BoolFact(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
