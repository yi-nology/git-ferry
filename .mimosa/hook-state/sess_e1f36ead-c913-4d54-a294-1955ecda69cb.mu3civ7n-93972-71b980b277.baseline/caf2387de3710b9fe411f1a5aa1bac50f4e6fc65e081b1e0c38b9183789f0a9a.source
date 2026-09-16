package converter

import (
	"testing"
	"time"

	"github.com/yi-nology/git-sync-core/model"
)

func TestToRepoInfo(t *testing.T) {
	now := time.Now()
	r := &model.Repo{
		ID:            1,
		Key:           "test-repo",
		Name:          "Test Repo",
		Platform:      "github",
		PlatformOwner: "testuser",
		PlatformRepo:  "test-repo",
		CloneURL:      "https://github.com/testuser/test-repo.git",
		SSHURL:        "git@github.com:testuser/test-repo.git",
		DefaultBranch: "main",
		Status:        "active",
		CreatedAt:     now,
	}

	result := ToRepoInfo(r)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.Key != "test-repo" {
		t.Errorf("Expected Key 'test-repo', got '%s'", result.Key)
	}

	if result.Name != "Test Repo" {
		t.Errorf("Expected Name 'Test Repo', got '%s'", result.Name)
	}

	if result.Platform != "github" {
		t.Errorf("Expected Platform 'github', got '%s'", result.Platform)
	}

	if result.PlatformOwner != "testuser" {
		t.Errorf("Expected PlatformOwner 'testuser', got '%s'", result.PlatformOwner)
	}

	if result.PlatformRepo != "test-repo" {
		t.Errorf("Expected PlatformRepo 'test-repo', got '%s'", result.PlatformRepo)
	}

	if result.CloneUrl != "https://github.com/testuser/test-repo.git" {
		t.Errorf("Expected CloneUrl 'https://github.com/testuser/test-repo.git', got '%s'", result.CloneUrl)
	}

	if result.SshUrl != "git@github.com:testuser/test-repo.git" {
		t.Errorf("Expected SshUrl 'git@github.com:testuser/test-repo.git', got '%s'", result.SshUrl)
	}

	if result.DefaultBranch != "main" {
		t.Errorf("Expected DefaultBranch 'main', got '%s'", result.DefaultBranch)
	}

	if result.Status != "active" {
		t.Errorf("Expected Status 'active', got '%s'", result.Status)
	}
}

func TestToRepoInfoNil(t *testing.T) {
	result := ToRepoInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToRepoInfoList(t *testing.T) {
	repos := []*model.Repo{
		{ID: 1, Key: "repo1", Name: "Repo 1"},
		{ID: 2, Key: "repo2", Name: "Repo 2"},
	}

	result := ToRepoInfoList(repos)

	if len(result) != 2 {
		t.Fatalf("Expected 2 repos, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first repo ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second repo ID 2, got %d", result[1].ID)
	}
}

func TestToRepoInfoListEmpty(t *testing.T) {
	result := ToRepoInfoList([]*model.Repo{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}
