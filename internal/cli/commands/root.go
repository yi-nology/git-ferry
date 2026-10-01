package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/cmdutil"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// Version 由 -ldflags 注入。
var Version = "dev"

// NewRootCmd 组装根命令与全部子命令。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "gitferry",
		Short:         "GitFerry CLI — Git 多平台同步/镜像/备份的命令行入口",
		Long:          "gitferry 是 GitFerry（git-ferry）的命令行客户端：仓库、同步任务、执行历史、运维巡检，输出统一 Envelope，面向人与 AI Agent。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&cmdutil.BaseURL, "base-url", "", "服务基址（默认 GITFERRY_BASE_URL 或 http://127.0.0.1:8890）")
	pf.StringVar(&cmdutil.Format, "format", "", "输出格式 json|table（Agent 场景用 json）")
	pf.BoolVar(&cmdutil.Debug, "debug", false, "调试输出")
	pf.BoolVar(&cmdutil.Yes, "yes", false, "跳过危险操作确认（仅脚本/自动化）")

	root.AddCommand(
		newAuthCmd(),
		newConfigCmd(),
		newSchemaCmd(),
		newAPICmd(),
		newRepoCmd(),
		newTaskCmd(),
		newHistoryCmd(),
		newOpsCmd(),
		newPlatformCmd(),
		newWebhookCmd(),
		newVersionCmd(),
	)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gitferry %s\n", Version)
		},
	}
}

// newClient 按全局 flags + 配置构造客户端。
func newClient() (*client.Client, *config.Config, error) {
	cfg, err := config.Effective()
	if err != nil {
		return nil, nil, err
	}
	if cmdutil.BaseURL != "" {
		cfg.BaseURL = cmdutil.BaseURL
	}
	if cmdutil.Format != "" {
		cfg.Format = cmdutil.Format
	}
	if cfg.Format == "" {
		cfg.Format = "json"
	}
	return client.New(cfg.BaseURL, cfg.APIKey), cfg, nil
}

// emit 输出 Envelope；失败时返回非 nil error 以便 cobra 退出码非 0。
func emit(env *output.Envelope, format string) error {
	if err := output.Print(os.Stdout, env, format); err != nil {
		return err
	}
	if !env.OK {
		return fmt.Errorf("%s", env.Error.Message)
	}
	return nil
}

// runWith 抽出「建客户端 → 执行 → 输出」公共流程。
func runWith(fn func(c *client.Client, cfg *config.Config, cmd *cobra.Command, args []string) (*output.Envelope, error)) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		c, cfg, err := newClient()
		if err != nil {
			_ = emit(output.Fail(0, err.Error(), ""), "json")
			os.Exit(1)
		}
		env, err := fn(c, cfg, cmd, args)
		if err != nil {
			_ = emit(output.Fail(0, err.Error(), ""), cfg.Format)
			os.Exit(1)
		}
		if err := emit(env, cfg.Format); err != nil {
			os.Exit(1)
		}
	}
}

// confirmDanger 危险操作：--yes 放行，否则拒绝并提示。
func confirmDanger(action string) *output.Envelope {
	if cmdutil.Yes {
		return nil
	}
	return output.Fail(409, "需要确认: "+action, cmdutil.SuggestYes)
}
