package git_sync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
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
		authz := string(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authz, "Bearer ") {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: bearer token required"})
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
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
		if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: token expired"})
			c.Abort()
			return
		}
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
		SetAuthRole(c, ParseRole(roleStr))
		c.Next(ctx)
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
func parseHS256JWT(token, secret string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("bad header")
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("bad header json")
	}
	if !strings.EqualFold(header.Alg, "HS256") {
		return nil, fmt.Errorf("unsupported alg %s", header.Alg)
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expect := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("bad signature encoding")
	}
	if !hmac.Equal(expect, got) {
		return nil, errors.New("signature mismatch")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("bad payload")
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errors.New("bad payload json")
	}
	// 收集 Extra 供自定义 claim 读取
	var raw map[string]any
	_ = json.Unmarshal(payloadJSON, &raw)
	claims.Extra = raw
	return &claims, nil
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
