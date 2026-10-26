package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "认证与本地凭据",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var token, baseURL string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "写入 API Key / 基址到配置文件",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if baseURL != "" {
				cfg.BaseURL = strings.TrimRight(baseURL, "/")
			}
			switch {
			case token != "":
				cfg.APIKey = token
			case os.Getenv("GITFERRY_TOKEN") != "":
				cfg.APIKey = os.Getenv("GITFERRY_TOKEN")
			case os.Getenv("GITFERRY_API_KEY") != "":
				cfg.APIKey = os.Getenv("GITFERRY_API_KEY")
			}
			if cfg.APIKey == "" {
				return fmt.Errorf("未提供 API Key：用 --token 或设置 GITFERRY_TOKEN")
			}
			if err := config.SaveAPIKey(cfg); err != nil {
				return err
			}
			return emit(output.Success(map[string]any{
				"config_path": mustConfigPath(),
				"base_url":    cfg.BaseURL,
				"has_api_key": true,
				"keychain":    true,
			}, nil), "json")
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "API Key（也可用 GITFERRY_TOKEN）")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "服务基址")
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看当前认证配置与服务连通状态",
		Run: func(cmd *cobra.Command, args []string) {
			c, cfg, err := newClient()
			if err != nil {
				_ = emit(output.Fail(0, err.Error(), ""), "json")
				os.Exit(1)
			}
			_, env := authStatus(c, cfg)
			// emit 对 !env.OK 返回 error ⇒ 退出码非 0(失败必须以失败告终)。
			if err := emit(env, "json"); err != nil {
				os.Exit(1)
			}
		},
	}
}

// authStatus 采集本地认证配置与服务连通状态,返回信息体与最终信封。
//
// **服务端报错(401/403/5xx)必须如实报失败**:以前这里无论成功失败都
// output.Success(...)⇒ 信封里永远 ok=true,脚本按 ok 判断就会把
// "密钥无效/服务不可达"当成"一切正常"。
//
// 拆成独立函数是为了能测:原实现里 os.Exit 会把测试进程带走。
func authStatus(c *client.Client, cfg *config.Config) (map[string]any, *output.Envelope) {
	info := map[string]any{
		"base_url":    cfg.BaseURL,
		"has_api_key": cfg.APIKey != "",
		"config_path": mustConfigPath(),
	}
	env, err := c.Get("/api/v1/system/status", nil)
	if err != nil {
		// 真·传输层错误(Do 把 HTTP 错误包成 Fail 信封,所以这里极少走到)。
		info["reachable"] = false
		info["error"] = err.Error()
		return info, output.Fail(0, err.Error(), "检查 GITFERRY_BASE_URL 与服务是否启动（GET /health）")
	}
	if !env.OK {
		msg, suggestion := "服务返回错误", ""
		code := 0
		if env.Error != nil {
			if env.Error.Message != "" {
				msg = env.Error.Message
			}
			suggestion = env.Error.Suggestion
			if n, ok := env.Error.Code.(int); ok {
				code = n
			}
		}
		info["reachable"] = false
		info["error"] = msg
		return info, output.Fail(code, msg, suggestion)
	}
	info["reachable"] = true
	info["system"] = env.Data
	return info, output.Success(info, nil)
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "清除本地配置中的 API Key",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := config.ClearAPIKey(cfg); err != nil {
				return err
			}
			return emit(output.Success(map[string]any{"logged_out": true}, nil), "json")
		},
	}
}

func mustConfigPath() string {
	p, err := config.Path()
	if err != nil {
		return "~/.config/gitferry/config.yaml"
	}
	return p
}
