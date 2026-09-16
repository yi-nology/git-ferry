package git_sync

// 身份上下文：内网/公网壳在鉴权中间件里写入已认证用户，
// handler 审计与后续权限判断统一从这里读取。
// core 不感知登录态，只落库 OperationLog.Actor 字符串。

const authUserKey = "auth_user"

// SetAuthUser 由壳层鉴权中间件调用，写入已认证用户名（SSO/网关头/自定义 IdP）。
func SetAuthUser(c appRequestContext, user string) {
	if user == "" || c == nil {
		return
	}
	c.Set(authUserKey, user)
}

// GetAuthUser 读取已认证用户名；未设置时返回空串。
func GetAuthUser(c appRequestContext) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Value(authUserKey).(string); ok {
		return v
	}
	return ""
}

// appRequestContext 最小依赖，便于测试；生产为 *app.RequestContext。
// Hertz 的 Value 参数类型是 any。
type appRequestContext interface {
	Set(key string, value any)
	Value(key any) any
}
