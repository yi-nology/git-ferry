//go:build !darwin

package config

import "errors"

// 非 macOS 平台：无系统 Keychain，统一走文件配置。
func loadSecret(account string) (string, error) { return "", errSecretNotFound }

func saveSecret(account, secret string) bool { return false }

func deleteSecret(account string) {}

// errSecretNotFound 非 macOS 平台没有 Keychain ⇒ 等价于"条目不存在"。
var errSecretNotFound = errors.New("keychain item not found")
