//go:build darwin

package config

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Keychain service name for macOS `security` CLI.
const keychainService = "gitferry-cli"

// loadSecret 优先从 OS Keychain 读取；不支持或不存在时回退空串（由调用方读文件配置）。
func loadSecret(account string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	//nolint:gosec // G204: account 仅限内部常量 "api_key"，非用户可控
	out, err := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", account, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// saveSecret 写入 OS Keychain（macOS）；其他平台或失败时返回 false，调用方写文件。
func saveSecret(account, secret string) bool {
	if runtime.GOOS != "darwin" || secret == "" {
		return false
	}
	// 先删旧项，忽略错误
	//nolint:gosec // G204: account 仅限内部常量
	_ = exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", account).Run()
	//nolint:gosec // G204: secret 经调用方注入，账户名非用户可控
	cmd := exec.Command("security", "add-generic-password",
		"-s", keychainService, "-a", account, "-w", secret)
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

// deleteSecret 从 Keychain 移除。
func deleteSecret(account string) {
	if runtime.GOOS != "darwin" {
		return
	}
	//nolint:gosec // G204: account 仅限内部常量
	_ = exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", account).Run()
}
