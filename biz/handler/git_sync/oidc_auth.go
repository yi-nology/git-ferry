package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	kitxjwt "github.com/oekit/kitx/jwt"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// OIDCConfig OIDC/JWT Bearer 鉴权配置(公网壳可选)。
// 支持 HS256 共享密钥模式:与 IdP/网关约定 secret,校验 exp/iss/aud。
// 生产更推荐在反向代理层做 OIDC,再用 SetAuthMiddlewareProvider 注入身份头。
type OIDCConfig struct {
	Enabled bool `yaml:"enabled"`
	// Secret HS256 共享密钥(必填;RS256 请走代理层)
	Secret string `yaml:"secret"`
	// Issuer 期望的 iss(空=不校验)
	Issuer string `yaml:"issuer"`
	// Audience 期望的 aud(空=不校验)
	Audience string `yaml:"audience"`
	// RoleClaim JWT 中角色声明键,默认 "role"
	RoleClaim string `yaml:"role_claim"`
	// UserClaim JWT 中用户名声明键,默认 "sub"
	UserClaim string `yaml:"user_claim"`
	// DefaultRole 无角色声明时的缺省角色,默认 readonly
	DefaultRole string `yaml:"default_role"`
}

var oidcCfg *OIDCConfig

// SetOIDCConfig 注册 OIDC 配置(壳层启动时调用)。
func SetOIDCConfig(cfg *OIDCConfig) {
	oidcCfg = cfg
}

// OIDCAuthMiddleware 校验 Authorization: Bearer <jwt>(HS256)。
// 未启用时返回 nil,调用方应回退默认 API Key 鉴权。
func OIDCAuthMiddleware() app.HandlerFunc {
	if oidcCfg == nil || !oidcCfg.Enabled || oidcCfg.Secret == "" {
		return nil
	}
	cfg := *oidcCfg
	return func(ctx context.Context, c *app.RequestContext) {
		authzHeader := string(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authzHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: bearer token required"})
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authzHeader, "Bearer "))
		claims, err := parseHS256JWT(token, cfg.Secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: " + err.Error()})
			c.Abort()
			return
		}
		if cfg.Issuer != "" && claims.Iss != cfg.Issuer {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: issuer mismatch"})
			c.Abort()
			return
		}
		if cfg.Audience != "" && !audienceHas(claims.Aud, cfg.Audience) {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: audience mismatch"})
			c.Abort()
			return
		}
		// exp 校验已内建于 kitx ParseSymmetric(RegisteredClaims 过期验证)——
		// 原手搓 `claims.Exp > 0 && now > claims.Exp` 删除,语义等价。
		userClaim := cfg.UserClaim
		if userClaim == "" {
			userClaim = "sub"
		}
		roleClaim := cfg.RoleClaim
		if roleClaim == "" {
			roleClaim = "role"
		}
		user := stringClaim(claims.Extra, userClaim)
		if user == "" {
			user = claims.Sub
		}
		roleStr := stringClaim(claims.Extra, roleClaim)
		if roleStr == "" {
			roleStr = cfg.DefaultRole
		}
		SetAuthUser(c, "oidc:"+user)
		role := ParseRole(roleStr)
		SetAuthRole(c, role)
		// 角色**跟着 ctx 往后走**:工具层要判角色,
		// 而进程内直调 core service 的入口(MCP / AI 助手)不过路由守卫。
		c.Next(authz.WithRole(ctx, role))
	}
}

// jwtClaims 最小 JWT 声明集。
type jwtClaims struct {
	Iss   string          `json:"iss"`
	Sub   string          `json:"sub"`
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Extra map[string]any  `json:"-"`
}

// parseHS256JWT 解析并校验 HS256 JWT 签名。
// 收编 oekit kitx/jwt 对称形态(kitx#86 落地物):alg 单算法白名单(关死 alg
// 混淆)、hmac.Equal 常量时间比较、exp 校验、密钥强度校验(RFC 7518 §3.2
// ≥32B,弱密钥 fail-closed)——44 行手搓面删除,这些坑上游单源。
// 行为变化:①过期令牌在 Parse 阶段即拒(原实现在中间件手搓,语义等价、
// 错误文案变);②弱密钥(<32B)配置的部署所有请求 401 且错误文案带下限说明
// (原实现照跑不拒)——安全侧收紧。
func parseHS256JWT(token, secret string) (*jwtClaims, error) {
	claims, err := kitxjwt.ParseSymmetric(token, []byte(secret))
	if err != nil {
		return nil, err
	}
	out := &jwtClaims{
		Iss:   claims.Issuer,
		Sub:   claims.Subject,
		Extra: claims.Extra,
	}
	if claims.ExpiresAt != nil {
		out.Exp = claims.ExpiresAt.Unix()
	}
	// aud 双形态映射(RFC 7519:string / list)——中间件 audienceHas 口径不变
	switch len(claims.Audience) {
	case 0:
	case 1:
		out.Aud, _ = json.Marshal(claims.Audience[0])
	default:
		out.Aud, _ = json.Marshal(claims.Audience)
	}
	return out, nil
}

func audienceHas(aud json.RawMessage, want string) bool {
	if len(aud) == 0 {
		return false
	}
	var s string
	if err := json.Unmarshal(aud, &s); err == nil {
		return s == want
	}
	var list []string
	if err := json.Unmarshal(aud, &list); err == nil {
		for _, a := range list {
			if a == want {
				return true
			}
		}
	}
	return false
}

func stringClaim(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok2 := v.(string); ok2 {
		return s
	}
	return fmt.Sprint(v)
}
