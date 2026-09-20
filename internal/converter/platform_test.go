package converter

import (
	"testing"
	"time"

	platformmodel "github.com/yi-nology/git-ferry/biz/model/platform"
	"github.com/yi-nology/git-ferry-core/model"
)

func TestToPlatformInfo(t *testing.T) {
	now := time.Now()
	p := &model.Platform{
		ID:             1,
		Key:            "github",
		Name:           "GitHub",
		Type:           "github",
		InstanceURL:    "https://github.com",
		APIURL:         "https://api.github.com",
		SkipTLSVerify:  false,
		CACertPath:     "",
		ProxyURL:       "",
		IsDefault:      true,
		Status:         "active",
		LastTestResult: "success",
		RepoCount:      10,
		CreatedAt:      now,
		LastTestAt:     &now,
		UpdatedAt:      now,
	}

	result := ToPlatformInfo(p)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.Key != "github" {
		t.Errorf("Expected Key 'github', got '%s'", result.Key)
	}

	if result.Name != "GitHub" {
		t.Errorf("Expected Name 'GitHub', got '%s'", result.Name)
	}

	if result.Type != "github" {
		t.Errorf("Expected Type 'github', got '%s'", result.Type)
	}

	if result.InstanceUrl != "https://github.com" {
		t.Errorf("Expected InstanceUrl 'https://github.com', got '%s'", result.InstanceUrl)
	}

	if result.ApiUrl != "https://api.github.com" {
		t.Errorf("Expected ApiUrl 'https://api.github.com', got '%s'", result.ApiUrl)
	}

	if result.SkipTlsVerify {
		t.Error("Expected SkipTlsVerify to be false")
	}

	if !result.IsDefault {
		t.Error("Expected IsDefault to be true")
	}

	if result.Status != "active" {
		t.Errorf("Expected Status 'active', got '%s'", result.Status)
	}

	if result.LastTestResult != "success" {
		t.Errorf("Expected LastTestResult 'success', got '%s'", result.LastTestResult)
	}

	if result.RepoCount != 10 {
		t.Errorf("Expected RepoCount 10, got %d", result.RepoCount)
	}
}

func TestToPlatformInfoNil(t *testing.T) {
	result := ToPlatformInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToPlatformList(t *testing.T) {
	platforms := []*model.Platform{
		{ID: 1, Key: "github", Name: "GitHub"},
		{ID: 2, Key: "gitlab", Name: "GitLab"},
	}

	result := ToPlatformList(platforms)

	if len(result) != 2 {
		t.Fatalf("Expected 2 platforms, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first platform ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second platform ID 2, got %d", result[1].ID)
	}
}

func TestToPlatformListEmpty(t *testing.T) {
	result := ToPlatformList([]*model.Platform{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}

func TestApplyPlatformUpdate(t *testing.T) {
	p := &model.Platform{
		ID:          1,
		Key:         "github",
		Name:        "GitHub",
		InstanceURL: "https://github.com",
		APIURL:      "https://api.github.com",
	}

	req := &platformmodel.UpdatePlatformReq{
		Name:        "GitHub Enterprise",
		InstanceUrl: "https://github.example.com",
		ApiUrl:      "https://api.github.example.com",
	}

	ApplyPlatformUpdate(p, req)

	if p.Name != "GitHub Enterprise" {
		t.Errorf("Expected Name 'GitHub Enterprise', got '%s'", p.Name)
	}

	if p.InstanceURL != "https://github.example.com" {
		t.Errorf("Expected InstanceURL 'https://github.example.com', got '%s'", p.InstanceURL)
	}

	if p.APIURL != "https://api.github.example.com" {
		t.Errorf("Expected APIURL 'https://api.github.example.com', got '%s'", p.APIURL)
	}
}

func TestApplyPlatformUpdatePartial(t *testing.T) {
	p := &model.Platform{
		ID:          1,
		Key:         "github",
		Name:        "GitHub",
		InstanceURL: "https://github.com",
		APIURL:      "https://api.github.com",
	}

	req := &platformmodel.UpdatePlatformReq{
		Name: "GitHub Enterprise",
		// Other fields empty - should not be updated
	}

	ApplyPlatformUpdate(p, req)

	if p.Name != "GitHub Enterprise" {
		t.Errorf("Expected Name 'GitHub Enterprise', got '%s'", p.Name)
	}

	// These should remain unchanged
	if p.InstanceURL != "https://github.com" {
		t.Errorf("Expected InstanceURL 'https://github.com', got '%s'", p.InstanceURL)
	}

	if p.APIURL != "https://api.github.com" {
		t.Errorf("Expected APIURL 'https://api.github.com', got '%s'", p.APIURL)
	}
}

func TestApplyPlatformUpdateNil(t *testing.T) {
	// Should not panic
	ApplyPlatformUpdate(nil, nil)
	ApplyPlatformUpdate(&model.Platform{}, nil)
	ApplyPlatformUpdate(nil, &platformmodel.UpdatePlatformReq{})
}
