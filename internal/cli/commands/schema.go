package commands

import (
	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/output"
	"github.com/yi-nology/git-ferry/internal/cli/schema"
)

// newSchemaCmd 提供 API 自省，帮助用户与 Agent 发现可用端点。
func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "API 自省：列出域 / 查找端点",
		Long:  "从内嵌 OpenAPI 解析。示例：gitferry schema list / gitferry schema show sync / gitferry schema show task.run",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "列出全部 API 域与端点数",
			Run: func(cmd *cobra.Command, args []string) {
				domains, err := schema.List()
				if err != nil {
					_ = emit(output.Fail(0, err.Error(), ""), "json")
					return
				}
				rows := make([]map[string]any, 0, len(domains))
				for _, d := range domains {
					rows = append(rows, map[string]any{
						"domain":    d.Name,
						"prefix":    d.Prefix,
						"endpoints": len(d.Endpoints),
					})
				}
				_ = emit(output.Success(rows, nil), cmdFlagFormat())
			},
		},
		&cobra.Command{
			Use:   "show <domain|keyword>",
			Short: "查看某域或按关键字匹配端点",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				eps, err := schema.Show(args[0])
				if err != nil {
					_ = emit(output.Fail(0, err.Error(), "先运行 gitferry schema list"), "json")
					return
				}
				_ = emit(output.Success(eps, nil), cmdFlagFormat())
			},
		},
	)
	return cmd
}

func cmdFlagFormat() string {
	if cmdutilFormat() != "" {
		return cmdutilFormat()
	}
	return "json"
}
