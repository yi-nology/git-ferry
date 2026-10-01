package output

import "encoding/json"

// Envelope 是 CLI 对 Agent/脚本的统一输出契约（对齐 gitlink-cli）。
type Envelope struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error *ErrorInfo  `json:"error,omitempty"`
	Meta  *Meta       `json:"meta,omitempty"`
}

type ErrorInfo struct {
	Code       interface{} `json:"code,omitempty"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion,omitempty"`
}

type Meta struct {
	Page       int `json:"page,omitempty"`
	Limit      int `json:"limit,omitempty"`
	TotalCount int `json:"total_count,omitempty"`
	TotalPages int `json:"total_pages,omitempty"`
}

func Success(data interface{}, meta *Meta) *Envelope {
	return &Envelope{OK: true, Data: data, Meta: meta}
}

func Fail(code interface{}, message, suggestion string) *Envelope {
	return &Envelope{
		OK:    false,
		Error: &ErrorInfo{Code: code, Message: message, Suggestion: suggestion},
	}
}

// FromServer 把服务端响应映射为 Envelope。兼容：
//  1. 标准壳 {code, message, data, timestamp}（internal/pkg/response）
//  2. 错误体 {code, message} / {status, message}
//  3. 裸 JSON（列表包装 list/tasks/items/... 会拍平到 data + meta）
func FromServer(status int, raw json.RawMessage) *Envelope {
	if len(raw) == 0 {
		if status >= 400 {
			return Fail(status, httpText(status), suggestFor(status))
		}
		return Success(nil, nil)
	}

	var obj map[string]json.RawMessage
	isObject := json.Unmarshal(raw, &obj) == nil

	if status >= 400 {
		return Fail(status, extractMessage(raw, obj, isObject), suggestFor(status))
	}

	if !isObject {
		// 裸数组/标量
		return Success(json.RawMessage(raw), nil)
	}

	// 标准壳：{code, message, data}
	if hasAny(obj, "data") && hasAny(obj, "code") {
		code := intFrom(obj["code"])
		if code != 0 && code != 200 {
			return Fail(code, messageFrom(obj), suggestFor(code))
		}
		data := interface{}(json.RawMessage(obj["data"]))
		// data 内层若仍是列表包装，继续拍平
		var inner map[string]json.RawMessage
		if json.Unmarshal(obj["data"], &inner) == nil {
			if k := firstPresent(inner, "list", "tasks", "items", "platforms", "repos", "runs"); k != "" {
				return Success(json.RawMessage(inner[k]), metaFrom(inner))
			}
		}
		return Success(data, metaFrom(obj))
	}

	// 纯错误体：有 message + (code|status) 且 code/status 非 2xx
	if !hasAny(obj, "list") && !hasAny(obj, "items") && !hasAny(obj, "tasks") &&
		!hasAny(obj, "runs") && !hasAny(obj, "platforms") && !hasAny(obj, "repos") &&
		hasAny(obj, "message") && (hasAny(obj, "code") || hasAny(obj, "status")) {
		code := intFrom(obj["code"])
		if code == 0 {
			code = intFrom(obj["status"])
		}
		if code != 0 && code != 200 {
			return Fail(code, messageFrom(obj), suggestFor(code))
		}
	}

	// 裸对象：识别列表包装，否则整体透传
	if k := firstPresent(obj, "list", "tasks", "items", "platforms", "repos", "runs"); k != "" {
		return Success(json.RawMessage(obj[k]), metaFrom(obj))
	}
	return Success(json.RawMessage(raw), nil)
}

func metaFrom(wrap map[string]json.RawMessage) *Meta {
	meta := &Meta{}
	if p, ok := wrap["pagination"]; ok {
		var pg struct {
			Page       int   `json:"page"`
			PageSize   int   `json:"page_size"`
			Limit      int   `json:"limit"`
			Total      int64 `json:"total"`
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
		}
		if json.Unmarshal(p, &pg) == nil {
			meta.Page = pg.Page
			meta.Limit = pg.PageSize
			if meta.Limit == 0 {
				meta.Limit = pg.Limit
			}
			if pg.Total != 0 {
				meta.TotalCount = int(pg.Total)
			} else {
				meta.TotalCount = int(pg.TotalCount)
			}
			meta.TotalPages = pg.TotalPages
		}
	}
	if v, ok := wrap["total"]; ok {
		meta.TotalCount = intFrom(v)
	}
	if v, ok := wrap["total_count"]; ok {
		meta.TotalCount = intFrom(v)
	}
	if meta.Page == 0 && meta.Limit == 0 && meta.TotalCount == 0 && meta.TotalPages == 0 {
		return nil
	}
	return meta
}

func firstPresent(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return k
		}
	}
	return ""
}

func hasAny(m map[string]json.RawMessage, key string) bool {
	_, ok := m[key]
	return ok
}

func intFrom(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}
	return 0
}

func messageFrom(obj map[string]json.RawMessage) string {
	for _, k := range []string{"message", "msg", "error"} {
		if raw, ok := obj[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
	}
	return ""
}

func extractMessage(raw json.RawMessage, obj map[string]json.RawMessage, isObject bool) string {
	if isObject {
		if msg := messageFrom(obj); msg != "" {
			return msg
		}
	}
	return string(raw)
}

func suggestFor(code int) string {
	switch code {
	case 401:
		return "请设置 GITFERRY_TOKEN 或运行 gitferry auth login"
	case 403:
		return "API Key 角色权限不足，请确认 GIT_SYNC_API_KEY_ROLE"
	case 404:
		return "资源不存在，先用对应 +list 查看真实 key/id"
	case 409:
		return "需要确认后重试，脚本场景可加 --yes"
	default:
		return ""
	}
}

func httpText(code int) string {
	switch code {
	case 400:
		return "bad request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not found"
	case 500:
		return "internal server error"
	default:
		return "request failed"
	}
}
