package service

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/yi-nology/git-platform-sdk/pkg/credential"
	"github.com/yi-nology/git-platform-sdk/provider"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// Auto-migrate the Repo model
	if err := db.AutoMigrate(&model.Repo{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}

// setupTestService creates a Service with test dependencies
func setupTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	// Set encryption key for tests
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	db := setupTestDB(t)
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("failed to create RepoDAO: %v", err)
	}
	platformDAO, err := dao.NewPlatformDAO(db)
	if err != nil {
		t.Fatalf("failed to create PlatformDAO: %v", err)
	}

	providerMgr := provider.NewManager(0)
	repoService := NewRepoService(repoDAO, platformDAO, providerMgr)

	svc := &Service{
		repos: repoService,
	}
	return svc, db
}

func TestCreateRepo_ValidGitHubURL(t *testing.T) {
	svc, db := setupTestService(t)
	ctx := context.Background()

	req := &model.CreateRepoRequest{
		Name:        "test-repo",
		RemoteURL:   "https://github.com/owner/repo.git",
		AccessToken: "ghp_testtoken123",
	}

	repo, err := svc.CreateRepo(ctx, req)
	if err != nil {
		t.Fatalf("CreateRepo failed: %v", err)
	}

	if repo.Name != "test-repo" {
		t.Errorf("expected name 'test-repo', got %q", repo.Name)
	}
	if repo.Platform != "github" {
		t.Errorf("expected platform 'github', got %q", repo.Platform)
	}
	if repo.PlatformOwner != "owner" {
		t.Errorf("expected owner 'owner', got %q", repo.PlatformOwner)
	}
	if repo.PlatformRepo != "repo" {
		t.Errorf("expected repo 'repo', got %q", repo.PlatformRepo)
	}
	if repo.Status != "active" {
		t.Errorf("expected status 'active', got %q", repo.Status)
	}
	if repo.Key == "" {
		t.Error("expected non-empty key")
	}

	// Verify the token is encrypted in the database
	var stored model.Repo
	db.Where("`key` = ?", repo.Key).First(&stored)
	if stored.AccessToken == "ghp_testtoken123" {
		t.Error("access token should be encrypted in database")
	}
}

func TestCreateRepo_ValidGitLabURL(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	req := &model.CreateRepoRequest{
		Name:        "gitlab-repo",
		RemoteURL:   "https://gitlab.com/group/project.git",
		AccessToken: "glpat-testtoken",
	}

	repo, err := svc.CreateRepo(ctx, req)
	if err != nil {
		t.Fatalf("CreateRepo failed: %v", err)
	}

	if repo.Platform != "gitlab" {
		t.Errorf("expected platform 'gitlab', got %q", repo.Platform)
	}
}

func TestCreateRepo_UnsupportedPlatform(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	req := &model.CreateRepoRequest{
		Name:        "unsupported-repo",
		RemoteURL:   "https://unknown-host.example.com/owner/repo.git",
		AccessToken: "some-token",
	}

	_, err := svc.CreateRepo(ctx, req)
	if err == nil {
		t.Fatal("expected error for unsupported platform, got nil")
	}

	if !errors.Is(err, provider.ErrPlatformNotSupported) {
		t.Errorf("expected ErrPlatformNotSupported in error chain, got: %v", err)
	}
}

func TestCreateRepo_InvalidURL(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	req := &model.CreateRepoRequest{
		Name:        "invalid-repo",
		RemoteURL:   "not-a-valid-url",
		AccessToken: "some-token",
	}

	_, err := svc.CreateRepo(ctx, req)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestCreateRepo_NestedGroupPath(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	// DetectPlatform 对已知平台会拆 (owner, rest);这里主要防手工 URL 解析回退
	// 再次把嵌套群组截成两段。
	req := &model.CreateRepoRequest{
		Name:        "nested",
		RemoteURL:   "https://gitlab.com/obs/sdk/server.git",
		AccessToken: "glpat-x",
	}

	repo, err := svc.CreateRepo(ctx, req)
	if err != nil {
		t.Fatalf("CreateRepo nested group failed: %v", err)
	}

	if repo.PlatformOwner != "obs" {
		t.Errorf("expected owner 'obs', got %q", repo.PlatformOwner)
	}
	// DetectPlatform 语义:第一段作 owner,其余整段作 repo path
	if repo.PlatformRepo != "sdk/server" {
		t.Errorf("expected repo path 'sdk/server', got %q", repo.PlatformRepo)
	}
}

func TestCreateRepo_SSHURL(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	req := &model.CreateRepoRequest{
		Name:        "ssh-repo",
		RemoteURL:   "git@github.com:owner/repo.git",
		AccessToken: "ghp_testtoken",
	}

	repo, err := svc.CreateRepo(ctx, req)
	if err != nil {
		t.Fatalf("CreateRepo with SSH URL failed: %v", err)
	}

	if repo.Platform != "github" {
		t.Errorf("expected platform 'github', got %q", repo.Platform)
	}
	if repo.PlatformOwner != "owner" {
		t.Errorf("expected owner 'owner', got %q", repo.PlatformOwner)
	}
	if repo.PlatformRepo != "repo" {
		t.Errorf("expected repo 'repo', got %q", repo.PlatformRepo)
	}
}

// TestCryptoManager_Integration verifies that encryption works end-to-end
// through the DAO layer (Create -> FindByKey should return decrypted token)
func TestCryptoManager_Integration(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	db := setupTestDB(t)
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("failed to create RepoDAO: %v", err)
	}

	originalToken := "ghp_supersecrettoken12345"
	repo := &model.Repo{
		Key:         "test-key-1",
		Name:        "test-repo",
		Platform:    "github",
		CloneURL:    "https://github.com/owner/repo.git",
		AccessToken: originalToken,
		Status:      "active",
	}

	// Create should encrypt the token
	if err := repoDAO.Create(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// FindByKey should return the decrypted token
	found, err := repoDAO.FindByKey("test-key-1")
	if err != nil {
		t.Fatalf("FindByKey failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find repo, got nil")
	}
	if found.AccessToken != originalToken {
		t.Errorf("expected decrypted token %q, got %q", originalToken, found.AccessToken)
	}
}

// TestCryptoManager_EncryptDecrypt_Roundtrip directly tests CryptoManager
func TestCryptoManager_Direct_EncryptDecrypt_Roundtrip(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef"
	cm := credential.NewCryptoManagerFromKey(key)

	plaintext := "my-secret-token-123"
	encrypted, err := cm.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Error("encrypted text should differ from plaintext")
	}

	decrypted, err := cm.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypt roundtrip failed: got %q, want %q", decrypted, plaintext)
	}
}
