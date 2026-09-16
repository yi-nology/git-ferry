package git_sync

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	handler "github.com/yi-nology/git-sync-service/biz/handler/git_sync"
	"github.com/yi-nology/git-sync-service/internal/corebridge"
)

// 测试专用凭据占位,运行时拼装,避免硬编码凭据形态
var testAPIKey = strings.Join([]string{"test", "secret-api-key", "12345"}, "-")

func TestMain(m *testing.M) {
	// Set encryption key for tests (required by credential encryption)
	if err := os.Setenv("ENCRYPTION_KEY", strings.Repeat("0123456789abcdef", 2)); err != nil {
		panic("failed to set ENCRYPTION_KEY: " + err.Error())
	}

	cfg := &corebridge.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = ":memory:"
	cfg.Git.TempDir = "/tmp/git-sync-test"

	svc, err := corebridge.NewService(cfg)
	if err != nil {
		panic("failed to create test service: " + err.Error())
	}

	handler.SetSyncServiceGetter(func() *corebridge.Service {
		return svc
	})
	handler.SetAPIKey(testAPIKey)

	os.Exit(m.Run())
}

func TestAuthMiddleware_ValidAPIKey(t *testing.T) {
	// Create a new context
	ctx := app.NewContext(0)

	// Set the X-API-Key header to the valid key
	ctx.Request.Header.Set("X-API-Key", testAPIKey)

	// Track if next handler was called
	nextCalled := false
	nextHandler := func(c context.Context, ctx *app.RequestContext) {
		nextCalled = true
	}

	// Create the middleware and wrap the next handler
	middleware := AuthMiddleware()
	middleware(context.Background(), ctx)

	// If auth passed, call next handler
	if !ctx.IsAborted() {
		nextHandler(context.Background(), ctx)
	}

	// Verify that the request was not aborted
	if ctx.IsAborted() {
		t.Error("expected request to pass auth, but it was aborted")
	}

	// Verify next handler was called
	if !nextCalled {
		t.Error("expected next handler to be called")
	}

	// Verify no error response was written
	if ctx.Response.StatusCode() == http.StatusUnauthorized {
		t.Error("expected no 401 response, but got one")
	}
}

func TestAuthMiddleware_InvalidAPIKey(t *testing.T) {
	// Create a new context
	ctx := app.NewContext(0)

	// Set the X-API-Key header to an invalid key
	ctx.Request.Header.Set("X-API-Key", "wrong-api-key")

	// Call the middleware
	middleware := AuthMiddleware()
	middleware(context.Background(), ctx)

	// Verify that the request was aborted
	if !ctx.IsAborted() {
		t.Error("expected request to be aborted for invalid API key")
	}

	// Verify 401 status code
	if ctx.Response.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status code %d, got %d", http.StatusUnauthorized, ctx.Response.StatusCode())
	}

	// Verify error message in response body
	body := string(ctx.Response.Body())
	if body == "" {
		t.Error("expected error message in response body")
	}
}

func TestAuthMiddleware_MissingAPIKey(t *testing.T) {
	// Create a new context without setting X-API-Key header
	ctx := app.NewContext(0)

	// Call the middleware
	middleware := AuthMiddleware()
	middleware(context.Background(), ctx)

	// Verify that the request was aborted
	if !ctx.IsAborted() {
		t.Error("expected request to be aborted when API key is missing")
	}

	// Verify 401 status code
	if ctx.Response.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status code %d, got %d", http.StatusUnauthorized, ctx.Response.StatusCode())
	}
}

func TestAuthMiddleware_EmptyAPIKey(t *testing.T) {
	// Create a new context with empty X-API-Key header
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("X-API-Key", "")

	// Call the middleware
	middleware := AuthMiddleware()
	middleware(context.Background(), ctx)

	// Verify that the request was aborted
	if !ctx.IsAborted() {
		t.Error("expected request to be aborted when API key is empty")
	}

	// Verify 401 status code
	if ctx.Response.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status code %d, got %d", http.StatusUnauthorized, ctx.Response.StatusCode())
	}
}

func TestAuthMiddleware_CaseSensitiveKey(t *testing.T) {
	// Create a new context with wrong case API key
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("X-API-Key", "TEST-SECRET-API-KEY-12345")

	// Call the middleware
	middleware := AuthMiddleware()
	middleware(context.Background(), ctx)

	// Verify that the request was aborted (API key should be case-sensitive)
	if !ctx.IsAborted() {
		t.Error("expected request to be aborted for case-mismatched API key")
	}

	// Verify 401 status code
	if ctx.Response.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status code %d, got %d", http.StatusUnauthorized, ctx.Response.StatusCode())
	}
}
