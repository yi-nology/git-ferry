// Package textutil 收敛文本/路径安全化与 CSV 导出的小工具,
// 供 handler 与运维导出共用,避免散落重复实现。
package textutil

import "strings"

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

// CSVEscape 转义 CSV 单元格(含逗号/引号/换行时加引号)。
func CSVEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
