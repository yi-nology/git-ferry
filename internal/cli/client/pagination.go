package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// PaginateAll 拉取全部分页并合并列表项。
// 约定：query 带 page/per_page（或 page/limit），响应 data 为数组
// 或 {list|tasks|items|...} 包装；当本页条数 < limit 时停止。
func (c *Client) PaginateAll(path string, params url.Values) ([]json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	limit := 50
	if v := params.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	} else if v := params.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	params.Set("per_page", strconv.Itoa(limit))
	params.Set("limit", strconv.Itoa(limit))

	var all []json.RawMessage
	for page := 1; page <= 100; page++ {
		params.Set("page", strconv.Itoa(page))
		env, err := c.Get(path, params)
		if err != nil {
			return nil, err
		}
		if !env.OK {
			return nil, fmt.Errorf("page %d: %s", page, env.Error.Message)
		}

		items := extractItems(env.Data)
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if len(items) < limit {
			break
		}
	}
	return all, nil
}

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
