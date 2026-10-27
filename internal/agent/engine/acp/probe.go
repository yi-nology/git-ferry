package acp

import (
	"context"
	"fmt"
	"github.com/oekit/pkg/errors"
	"os/exec"
	"strings"
	"time"

	"github.com/oekit/concurrency/async"
)

// ProbeStdio 对 stdio transport 做一次**真实握手**。
//
// 「命令在不在」≠「它会说 ACP」—— 后者才是「测试连接」要回答的问题。
// 做法:启动命令 → initialize → 读 initialize 响应 → 关掉。
// **不发任何 prompt**(不消耗模型额度,也不在主机上跑任务)。
func ProbeStdio(ctx context.Context, cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("配置为空")
	}
	fields := splitCommand(cfg.Command)
	if len(fields) == 0 {
		return "", errors.New("command 不能为空")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pctx, pcancel := context.WithCancel(context.Background())
	defer pcancel()
	cmd := exec.CommandContext(pctx, fields[0], fields[1:]...) //nolint:gosec // G204:命令来自部署配置
	cmd.Dir = cfg.Cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", errors.Wrap(err, "agent stdin 管道失败")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "agent stdout 管道失败")
	}
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("启动 agent 失败(%s)", cfg.Command))
	}

	c := newClient(stdin, stderr)
	async.GoSafe(func() {
		reader := newLineReader(stdout)
		for {
			m, rerr := reader.next()
			if rerr != nil || m == nil {
				break
			}
			c.dispatch(m)
		}
		c.close()
	})

	var res struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := c.call(ctx, "initialize", cfg.initParams(), &res); err != nil {
		pcancel()
		return "", errors.Wrap(err, "ACP initialize 失败", "stderr", strings.TrimSpace(stderr.String()))
	}
	pcancel()

	name := res.AgentInfo.Title
	if name == "" {
		name = res.AgentInfo.Name
	}
	if name == "" {
		name = fields[0]
	}
	return fmt.Sprintf("%s %s(ACP 协议 v%d)", name, res.AgentInfo.Version, res.ProtocolVersion), nil
}
