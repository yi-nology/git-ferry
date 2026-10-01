package commands

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// runFunc 是 Shortcuts 的执行体签名。
type runFunc func(c *client.Client, cfg *config.Config, cmd *cobra.Command, args []string) (*output.Envelope, error)

// sc 声明一个 `+verb` Shortcut，并挂上常用 flags。
func sc(name, short string, fn runFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Run:   runWith(fn),
	}
	f := cmd.Flags()
	f.String("key", "", "资源 key（仓库/任务/平台）")
	f.String("task", "", "任务 key")
	f.String("run-id", "", "执行记录 ID")
	f.String("name", "", "名称（任务名/bundle 等）")
	f.String("keyword", "", "关键字过滤")
	f.String("platform", "", "平台 key")
	f.String("status", "", "状态过滤")
	f.String("max-seconds", "", "RPO 阈值（秒）")
	f.String("page", "", "页码")
	f.String("limit", "", "每页条数")
	f.Bool("all", false, "拉取全部分页（仅列表类命令）")
	f.Bool("dry-run", false, "预览将要执行的动作，不落盘（仅批量/危险命令）")
	f.Bool("with-drift", false, "健康评分时并行做漂移检测并折入 safety 维度")
	f.Bool("csv", false, "以 CSV 输出（审计导出/批量结果）")
	f.String("action", "", "按 action 过滤（审计）")
	// 任务字段
	f.String("source-repo", "", "源仓库 key")
	f.String("target-repo", "", "目标仓库 key")
	f.String("source-branch", "", "源分支")
	f.String("target-branch", "", "目标分支")
	f.String("sync-mode", "", "同步模式 all|single")
	f.String("cron", "", "cron 表达式")
	f.String("clone-url", "", "克隆地址（repo +create）")
	f.String("task-keys", "", "逗号分隔任务 key 列表（批量）")
	f.Bool("enabled", true, "是否启用（task +update）")
	f.Bool("git-tags", false, "同步 tags")
	f.Bool("git-force", false, "允许 force push")
	f.Bool("git-prune", false, "git prune")
	f.Bool("git-lfs", false, "同步 LFS")
	f.Bool("git-push-prune", false, "push --prune")
	f.String("include-branches", "", "分支 glob 白名单，逗号分隔（如 main,release/*）")
	f.String("exclude-ref-patterns", "", "忽略的 ref glob，逗号分隔（默认 refs/pull/* 等）")
	f.String("force-push-policy", "", "allow|block|backup_on_demand")
	return cmd
}

func flagStr(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func requireFlag(cmd *cobra.Command, name string) (string, error) {
	v := flagStr(cmd, name)
	if v == "" {
		return "", fmt.Errorf("缺少必填参数 --%s", name)
	}
	return v, nil
}

// setIf 把 flag 值拷入 query（flag 名与 query 名不同时用 to）。
func setIf(cmd *cobra.Command, q url.Values, flagName, queryName string) {
	if v := flagStr(cmd, flagName); v != "" {
		q.Set(queryName, v)
	}
}

// splitCSV 逗号分隔并去空白。
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
