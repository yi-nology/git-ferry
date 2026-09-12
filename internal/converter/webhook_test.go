package converter

import (
	"testing"
	"time"

	"github.com/yi-nology/git-sync-core/model"
)

func TestToRuleInfo(t *testing.T) {
	now := time.Now()
	r := &model.WebhookRule{
		ID:            1,
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		Tasks: []model.WebhookRuleTask{
			{TaskKey: "task1"},
			{TaskKey: "task2"},
		},
		MinInterval: 60,
		Enabled:     true,
		Description: "Test webhook rule",
		CreatedAt:   now,
	}

	result := ToRuleInfo(r)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.Name != "Test Rule" {
		t.Errorf("Expected Name 'Test Rule', got '%s'", result.Name)
	}

	if result.RepoKey != "test-repo" {
		t.Errorf("Expected RepoKey 'test-repo', got '%s'", result.RepoKey)
	}

	if result.EventType != "push" {
		t.Errorf("Expected EventType 'push', got '%s'", result.EventType)
	}

	if result.BranchPattern != "main" {
		t.Errorf("Expected BranchPattern 'main', got '%s'", result.BranchPattern)
	}

	if result.Action != "sync" {
		t.Errorf("Expected Action 'sync', got '%s'", result.Action)
	}

	if result.SyncTaskKeys != "task1,task2" {
		t.Errorf("Expected SyncTaskKeys 'task1,task2', got '%s'", result.SyncTaskKeys)
	}

	if result.MinInterval != 60 {
		t.Errorf("Expected MinInterval 60, got %d", result.MinInterval)
	}

	if !result.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if result.Description != "Test webhook rule" {
		t.Errorf("Expected Description 'Test webhook rule', got '%s'", result.Description)
	}
}

func TestToRuleInfoNil(t *testing.T) {
	result := ToRuleInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToRuleInfoList(t *testing.T) {
	rules := []*model.WebhookRule{
		{ID: 1, Name: "Rule 1"},
		{ID: 2, Name: "Rule 2"},
	}

	result := ToRuleInfoList(rules)

	if len(result) != 2 {
		t.Fatalf("Expected 2 rules, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first rule ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second rule ID 2, got %d", result[1].ID)
	}
}

func TestToRuleInfoListEmpty(t *testing.T) {
	result := ToRuleInfoList([]*model.WebhookRule{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}

func TestToEventInfo(t *testing.T) {
	now := time.Now()
	e := &model.WebhookEvent{
		ID:           1,
		EventID:      "evt-123",
		RepoKey:      "test-repo",
		EventType:    "push",
		Source:       "github",
		ActorName:    "testuser",
		Branch:       "main",
		CommitSHA:    "abc123",
		Status:       "processed",
		ErrorMessage: "",
		ProcessedAt:  &now,
		CreatedAt:    now,
	}

	result := ToEventInfo(e)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.EventId != "evt-123" {
		t.Errorf("Expected EventId 'evt-123', got '%s'", result.EventId)
	}

	if result.RepoKey != "test-repo" {
		t.Errorf("Expected RepoKey 'test-repo', got '%s'", result.RepoKey)
	}

	if result.EventType != "push" {
		t.Errorf("Expected EventType 'push', got '%s'", result.EventType)
	}

	if result.Source != "github" {
		t.Errorf("Expected Source 'github', got '%s'", result.Source)
	}

	if result.ActorName != "testuser" {
		t.Errorf("Expected ActorName 'testuser', got '%s'", result.ActorName)
	}

	if result.Branch != "main" {
		t.Errorf("Expected Branch 'main', got '%s'", result.Branch)
	}

	if result.CommitSha != "abc123" {
		t.Errorf("Expected CommitSha 'abc123', got '%s'", result.CommitSha)
	}

	if result.Status != "processed" {
		t.Errorf("Expected Status 'processed', got '%s'", result.Status)
	}
}

func TestToEventInfoNil(t *testing.T) {
	result := ToEventInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToEventInfoList(t *testing.T) {
	events := []*model.WebhookEvent{
		{ID: 1, EventID: "evt-1"},
		{ID: 2, EventID: "evt-2"},
	}

	result := ToEventInfoList(events)

	if len(result) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first event ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second event ID 2, got %d", result[1].ID)
	}
}

func TestToEventInfoListEmpty(t *testing.T) {
	result := ToEventInfoList([]*model.WebhookEvent{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}
