package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

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
			info := map[string]any{
				"base_url":    cfg.BaseURL,
				"has_api_key": cfg.APIKey != "",
				"config_path": mustConfigPath(),
			}
			env, err := c.Get("/api/v1/system/status", nil)
			if err != nil {
				info["reachable"] = false
				info["error"] = err.Error()
				_ = emit(output.Success(info, nil), "json")
				return
			}
			info["reachable"] = env.OK
			if !env.OK {
				info["error"] = env.Error
			} else {
				info["system"] = env.Data
			}
			if err := emit(output.Success(info, nil), "json"); err != nil {
				os.Exit(1)
			}
		},
	}
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
