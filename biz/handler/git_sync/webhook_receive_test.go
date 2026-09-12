package git_sync

import (
	"os"
	"testing"

	"github.com/yi-nology/git-sync-service/internal/corebridge"
)

const testAPIKey = "test-secret-api-key-12345"

func TestMain(m *testing.M) {
	if err := os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef"); err != nil {
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
