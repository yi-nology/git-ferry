package git_sync

import (
	"os"
	"testing"

	"github.com/yi-nology/git-sync-core"
)

const testAPIKey = "test-secret-api-key-12345"

func TestMain(m *testing.M) {
	// Set encryption key for tests (required by credential encryption)
	if err := os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef"); err != nil {
		panic("failed to set ENCRYPTION_KEY: " + err.Error())
	}

	// Create a test service with a known API key using SQLite in-memory database
	cfg := &sync.Config{}
	cfg.Server.APIKey = testAPIKey
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = ":memory:"
	cfg.Git.TempDir = "/tmp/git-sync-test"

	svc, err := sync.NewService(cfg)
	if err != nil {
		panic("failed to create test service: " + err.Error())
	}

	SetSyncServiceGetter(func() *sync.Service {
		return svc
	})

	os.Exit(m.Run())
}
