package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// PaginateAll 拉取全部分页并合并列表项,并返回**服务端给的真实总数**。
//
// **页长参数名必须与服务端一致**(`/api/v1/repos`、`/api/v1/sync/tasks` 都是
// `page_size`):发成 per_page/limit ⇒ 服务端静默忽略 ⇒ 回落默认页长(10)
// ⇒ 客户端以为拿到了全部,实际只拿到第一页。
//
// 结束条件**读服务端返回的总数/总页数**(信封 meta),不再拿"本页条数 < 我自己
// 的 limit"去猜 —— limit 与服务端实际页长不等时,这个判断会把"还有下一页"
// 误判成"已到最后一页"。
//
// 第三个返回值是服务端的真实总数(来自 meta),调用方**必须**用它,不能用
// len(items) 自算 —— 那会把"被截断的 10 条"谎报成总数。
func (c *Client) PaginateAll(path string, params url.Values) ([]json.RawMessage, *output.Meta, error) {
	if params == nil {
		params = url.Values{}
	}
	limit := 50
	if v := params.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	} else if v := params.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	} else if v := params.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	// 服务端认 page_size(per_page/limit 一并带上:有的端点用的是后者)。
	params.Set("page_size", strconv.Itoa(limit))
	params.Set("per_page", strconv.Itoa(limit))
	params.Set("limit", strconv.Itoa(limit))

	var all []json.RawMessage
	var meta *output.Meta
	for page := 1; page <= 1000; page++ {
		params.Set("page", strconv.Itoa(page))
		env, err := c.Get(path, params)
		if err != nil {
			return nil, nil, err
		}
		if !env.OK {
			return nil, nil, fmt.Errorf("page %d: %s", page, env.Error.Message)
		}

		items := extractItems(env.Data)
		if len(items) == 0 {
			break
		}
		all = append(all, items...)

		// 优先按服务端的分页信息判断是否还有下一页
		if env.Meta != nil {
			meta = env.Meta
			if env.Meta.TotalPages > 0 {
				if page >= env.Meta.TotalPages {
					break
				}
			} else if env.Meta.TotalCount > 0 && len(all) >= env.Meta.TotalCount {
				break
			}
			continue
		}
		// 服务端没给分页信息 ⇒ 退回"本页条数 < 页长"的判断
		if len(items) < limit {
			break
		}
	}
	return all, meta, nil
}

// ExtractItems 从响应 data 里取出列表项(数组或 {list|tasks|items|platforms|repos|runs} 包装)。
//
// 导出给命令层复用:命令层要自己解析列表时,**必须走同一份解析**
// (各处各写一套就会出现"CLI 以为取到了、服务端其实换了个形状"的静默偏差)。
func ExtractItems(data interface{}) []json.RawMessage { return extractItems(data) }

func extractItems(data interface{}) []json.RawMessage {
	raw, ok := data.(json.RawMessage)
	if !ok {
		// 已是数组
		if arr, ok := data.([]interface{}); ok {
			out := make([]json.RawMessage, 0, len(arr))
			for _, it := range arr {
				b, _ := json.Marshal(it)
				out = append(out, b)
			}
			return out
		}
		return nil
	}
	// 数组
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	// 对象包装
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err == nil {
		for _, k := range []string{"list", "tasks", "items", "platforms", "repos", "runs"} {
			if v, ok := wrap[k]; ok {
				var inner []json.RawMessage
				if err := json.Unmarshal(v, &inner); err == nil {
					return inner
				}
			}
		}
	}
	// 单对象
	if len(raw) > 0 && raw[0] == '{' {
		return []json.RawMessage{raw}
	}
	return nil
}
