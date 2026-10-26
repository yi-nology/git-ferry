package acp

import (
	"os"
	"strings"
)

// 服务密钥的摘除规则。
//
// 外部 agent 子进程默认**继承父进程环境**(Go 的 exec.Cmd.Env 为 nil ⇒ 用 os.Environ())。
// 不处理的话,agent 能读到:
//   - ENCRYPTION_KEY —— **凭据加密主密钥**,可解密库里存的所有 git 凭据;
//   - GIT_SYNC_SERVER_API_KEY —— admin 角色的主 API Key,可绕开 MCP 的只读边界
//     直接调本服务的写接口;
//   - GITFERRY_AI_MCP_API_KEY —— 挂 MCP 用的 key。
//
// ⇒ MCP 的「只读角色」只是**纵深防御**,不是边界:真正的边界是 agent 的
// 操作系统级权限(用户 / 容器)。环境变量这一层必须先堵住。
var (
	scrubExactEnv = map[string]bool{
		"ENCRYPTION_KEY": true,
	}
	scrubEnvPrefixes = []string{
		"GIT_SYNC_",
		"GITFERRY_",
	}
)

// scrubEnv 构造外部 agent 子进程的环境变量:摘掉**本服务的密钥**,其余保留。
//
// **不能一刀切清空环境** —— agent 自己的 provider 鉴权(ANTHROPIC_API_KEY 等)
// 也要用这些变量,清空等于让 agent 跑不起来。
// 所以只精准摘掉上面那几类。
func scrubEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if scrubExactEnv[name] {
			continue
		}
		skip := false
		for _, p := range scrubEnvPrefixes {
			if strings.HasPrefix(name, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// scrubbedEnv 当前进程环境经摘除后的结果(给子进程用)。
func scrubbedEnv() []string { return scrubEnv(os.Environ()) }
