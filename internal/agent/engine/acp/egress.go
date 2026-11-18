package acp

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/oagentkit/guardx/egressproxy"
	"github.com/oekit/logx"
)

// 外部 agent / terminal 子进程的外联围栏（opt-in）。
//
// 配置（环境变量，运维面）：
//   - GITFERRY_EGRESS_ALLOW_HOSTS=api.anthropic.com,*.openai.com
//     非空才启用；空 = 不装代理（保持现状）。
//   - GITFERRY_EGRESS_ALLOW_PORTS=443,80（可选；缺省 443/80）
//
// 语义边界：HTTP_PROXY/HTTPS_PROXY 只对**遵守代理环境变量**的客户端生效，
// 子进程仍可绕开代理直连 —— 这是 guardx 代理档的真实能力边界，不声称
// 网络隔离。强隔离仍需容器/网络命名空间。
var (
	egressOnce sync.Once
	egressEnv  []string
	egressErr  error
)

// egressProxyEnv 懒启动本地出口代理并返回注入子进程的代理环境变量。
// 未配置返回 (nil,nil)；配置了但启动失败返回 error（调用方 fail-closed）。
func egressProxyEnv() ([]string, error) {
	egressOnce.Do(func() {
		hosts := splitList(os.Getenv("GITFERRY_EGRESS_ALLOW_HOSTS"))
		if len(hosts) == 0 {
			return
		}
		ports := parsePorts(os.Getenv("GITFERRY_EGRESS_ALLOW_PORTS"))
		proxy, err := egressproxy.New(egressproxy.Policy{
			AllowHosts: hosts,
			AllowPorts: ports,
		})
		if err != nil {
			egressErr = err
			logx.Error("egressproxy: 策略非法,拒绝启动外部 agent", "error", err)
			return
		}
		addr, err := proxy.Start()
		if err != nil {
			egressErr = err
			logx.Error("egressproxy: 启动失败,拒绝启动外部 agent", "error", err)
			return
		}
		logx.Info("egressproxy started", "addr", addr, "hosts", hosts)
		egressEnv = []string{
			"HTTP_PROXY=" + addr,
			"HTTPS_PROXY=" + addr,
			"ALL_PROXY=" + addr,
			"NO_PROXY=127.0.0.1,localhost",
		}
	})
	return egressEnv, egressErr
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePorts(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return []int{443, 80}
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 65535 {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return []int{443, 80}
	}
	return out
}
