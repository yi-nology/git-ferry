//go:build !darwin

package config

// 非 macOS 平台：无系统 Keychain，统一走文件配置。
func loadSecret(account string) string { return "" }

func saveSecret(account, secret string) bool { return false }

func deleteSecret(account string) {}
