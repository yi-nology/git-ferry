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

// newAPICmd 是 Raw API 逃生舱：Shortcuts 未覆盖的端点直通。
func newAPICmd() *cobra.Command {
	var body string
	var query []string

	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH>",
		Short: "Raw API 调用（GET/POST/PUT/DELETE）",
		Long:  "直通 GitFerry REST。示例：gitferry api GET /api/v1/system/status --format json",
		Args:  cobra.ExactArgs(2),
		Run: runWith(func(c *client.Client, cfg *config.Config, cmd *cobra.Command, args []string) (*output.Envelope, error) {
			method := strings.ToUpper(args[0])
			path := args[1]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			q := url.Values{}
			for _, kv := range query {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return nil, fmt.Errorf("无效 query 参数 %q（应为 k=v）", kv)
				}
				q.Add(k, v)
			}
			var payload interface{}
			if body != "" {
				payload = body
			}
			return c.Do(method, path, payload, q)
		}),
	}
	cmd.Flags().StringVar(&body, "body", "", "JSON 请求体")
	cmd.Flags().StringArrayVar(&query, "query", nil, "查询参数 k=v，可重复")
	return cmd
}
