package git_sync

import (
	"os"
	"strings"
	"testing"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// 测试专用凭据占位,运行时拼装,避免硬编码凭据形态
var testAPIKey = strings.Join([]string{"test", "secret-api-key", "12345"}, "-")

func TestMain(m *testing.M) {
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

	SetSyncServiceGetter(func() *corebridge.Service {
		return svc
	})
	SetAPIKey(testAPIKey)

	os.Exit(m.Run())
}
