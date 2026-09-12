package git_sync

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware_UsesProviderWhenSet(t *testing.T) {
	SetAuthMiddlewareProvider(func() app.HandlerFunc {
		return func(ctx context.Context, c *app.RequestContext) {
			c.Set("via", "provider")
			c.Next(ctx)
		}
	})
	t.Cleanup(func() { SetAuthMiddlewareProvider(nil) })

	c := app.NewContext(0)
	called := false
	c.SetHandlers([]app.HandlerFunc{func(ctx context.Context, rc *app.RequestContext) {
		called = true
	}})
	AuthMiddleware()(context.Background(), c)
	require.True(t, called)
	require.Equal(t, "provider", c.Value("via"))
}

func TestAuthMiddleware_DefaultAPIKeyWhenNoProvider(t *testing.T) {
	SetAuthMiddlewareProvider(nil)

	c := app.NewContext(0)
	c.Request.Header.Set("X-API-Key", testAPIKey)
	passed := false
	c.SetHandlers([]app.HandlerFunc{func(ctx context.Context, rc *app.RequestContext) {
		passed = true
	}})
	AuthMiddleware()(context.Background(), c)
	require.True(t, passed)

	// wrong key → 401
	c2 := app.NewContext(0)
	c2.Request.Header.Set("X-API-Key", "wrong")
	AuthMiddleware()(context.Background(), c2)
	require.Equal(t, http.StatusUnauthorized, c2.Response.StatusCode())
}
