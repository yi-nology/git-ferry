package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/cmdutil"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "本地配置管理（~/.config/gitferry/config.yaml）",
	}
	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigListCmd(),
		newConfigPathCmd(),
	)
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var baseURL, token, format string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "初始化配置（可交互式指定 base_url / api_key / format）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := &config.Config{
				BaseURL: config.DefaultBaseURL,
				Format:  "json",
			}
			if baseURL != "" {
				cfg.BaseURL = strings.TrimRight(baseURL, "/")
			}
			if token != "" {
				cfg.APIKey = token
			}
			if format != "" {
				cfg.Format = format
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			return emit(output.Success(map[string]any{
				"config_path": mustConfigPath(),
				"base_url":    cfg.BaseURL,
				"format":      cfg.Format,
				"has_api_key": cfg.APIKey != "",
			}, nil), "json")
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "服务基址")
	cmd.Flags().StringVar(&token, "token", "", "API Key")
	cmd.Flags().StringVar(&format, "format", "", "默认输出格式 json|table|yaml")
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "读取配置项（base_url|api_key|format）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Effective()
			if err != nil {
				return err
			}
			key := args[0]
			var val any
			switch key {
			case "base_url":
				val = cfg.BaseURL
			case "format":
				val = cfg.Format
			case "api_key":
				if cfg.APIKey == "" {
					val = ""
				} else {
					val = "***"
				}
			default:
				return fmt.Errorf("unknown key %q (base_url|api_key|format)", key)
			}
			return emit(output.Success(map[string]any{key: val}, nil), "json")
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "写入配置项（base_url|api_key|format）",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key, val := args[0], args[1]
			switch key {
			case "base_url":
				cfg.BaseURL = strings.TrimRight(val, "/")
			case "api_key":
				cfg.APIKey = val
			case "format":
				if val != "json" && val != "table" && val != "yaml" {
					return fmt.Errorf("format must be json|table|yaml")
				}
				cfg.Format = val
			default:
				return fmt.Errorf("unknown key %q (base_url|api_key|format)", key)
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			return emit(output.Success(map[string]any{"saved": true, "key": key}, nil), "json")
		},
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出当前生效配置（api_key 脱敏）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Effective()
			if err != nil {
				return err
			}
			key := "***"
			if cfg.APIKey == "" {
				key = ""
			}
			return emit(output.Success(map[string]any{
				"config_path": mustConfigPath(),
				"base_url":    cfg.BaseURL,
				"format":      cfg.Format,
				"api_key":     key,
			}, nil), cmdFlagFormat())
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "打印配置文件路径",
		Run: func(cmd *cobra.Command, args []string) {
			_ = emit(output.Success(map[string]any{"path": mustConfigPath()}, nil), "json")
		},
	}
}

func cmdutilFormat() string {
	return cmdutil.Format
}
