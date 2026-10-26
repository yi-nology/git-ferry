//go:build darwin

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Keychain service name for macOS `security` CLI.
const keychainService = "gitferry-cli"

// errSecretNotFound Keychain 里**没有**这个条目(= 用户从没配过)。
//
// 与"读不到"严格区分：读不到(锁住的 keychain / security 不可用 / 别的用户)
// 意味着**用户可能配过**,只是这个环境拿不到 ⇒ 提示不能说"请去设置 token"
// (那会把人引向错误的修法),要说"本机读不到已保存的密钥"。
var errSecretNotFound = errors.New("keychain item not found")

// securityNotFoundExitCode `security find-generic-password` 找不到条目时的退出码。
//
// 实测:条目/账号不存在 ⇒ 44;成功 ⇒ 0;其它(锁住、不可访问)⇒ 其它非 0。
const securityNotFoundExitCode = 44

// loadSecret 从 OS Keychain 读取。
//
// 返回 (值, 错误)：错误分两类 ——
//   - errSecretNotFound:条目不存在(用户没配过)
//   - 其它错误:**读不到**(keychain 锁住 / 不可访问 / security 不在 PATH)
//     ⇒ 这时不能断言"用户没配",提示必须区分开。
func loadSecret(account string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errSecretNotFound
	}
	//nolint:gosec // G204: account 仅限内部常量 "api_key"，非用户可控
	out, err := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", account, "-w").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == securityNotFoundExitCode {
			return "", errSecretNotFound
		}
		return "", fmt.Errorf("读取 macOS Keychain 失败: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// saveSecret 写入 OS Keychain（macOS）；其他平台或失败时返回 false，调用方写文件。
//
// **删之前先备份旧值**：`security add-generic-password` 在条目已存在时会失败
// ⇒ 必须先删；但"先删后加"在 add 失败（keychain 锁住 / security 不可用）时
// **旧密钥就凭空消失了**（用户的凭据被静默删掉，而调用方以为写成功了）。
// ⇒ 删之前把旧值读出来，add 失败就写回去。
func saveSecret(account, secret string) bool {
	if runtime.GOOS != "darwin" || secret == "" {
		return false
	}
	old, _ := loadSecret(account)
	// 先删旧项，忽略错误
	//nolint:gosec // G204: account 仅限内部常量
	_ = exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", account).Run()
	//nolint:gosec // G204: secret 经调用方注入，账户名非用户可控
	cmd := exec.Command("security", "add-generic-password",
		"-s", keychainService, "-a", account, "-w", secret)
	cmd.Env = os.Environ()
	if cmd.Run() == nil {
		return true
	}
	// 写回旧值（尽力而为）：别让一次失败把用户已存的密钥删掉。
	if old != "" {
		rb := exec.Command("security", "add-generic-password",
			"-s", keychainService, "-a", account, "-w", old)
		rb.Env = os.Environ()
		_ = rb.Run()
	}
	return false
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
