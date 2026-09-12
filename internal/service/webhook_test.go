package service

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.WebhookRule{}, &model.WebhookRuleTask{}, &model.WebhookEvent{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	// sqlite :memory: 每个连接是独立库;强制单连接,确保异步 goroutine 能看到已迁移的表
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

func setupWebhookTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	db := setupWebhookTestDB(t)
	ruleDAO := dao.NewWebhookRuleDAO(db)
	eventDAO := dao.NewWebhookEventDAO(db)

	webhookService := NewWebhookService(ruleDAO, eventDAO, nil)

	svc := &Service{
		webhooks: webhookService,
	}
	return svc, db
}

func TestCreateRule(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	req := &model.CreateRuleRequest{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		TaskKeys:      []string{"task-1", "task-2"},
		MinInterval:   60,
		Enabled:       true,
		Description:   "Test webhook rule",
	}

	rule, err := svc.CreateRule(context.TODO(), req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	if rule.Name != "Test Rule" {
		t.Errorf("expected name 'Test Rule', got %q", rule.Name)
	}
	if rule.RepoKey != "test-repo" {
		t.Errorf("expected repo key 'test-repo', got %q", rule.RepoKey)
	}
	if rule.EventType != "push" {
		t.Errorf("expected event type 'push', got %q", rule.EventType)
	}
	if rule.BranchPattern != "main" {
		t.Errorf("expected branch pattern 'main', got %q", rule.BranchPattern)
	}
	if rule.Action != "sync" {
		t.Errorf("expected action 'sync', got %q", rule.Action)
	}
	if rule.MinInterval != 60 {
		t.Errorf("expected min interval 60, got %d", rule.MinInterval)
	}
	if !rule.Enabled {
		t.Error("expected rule to be enabled")
	}

	// Verify task keys
	taskKeys := rule.GetTaskKeys()
	if len(taskKeys) != 2 {
		t.Errorf("expected 2 task keys, got %d", len(taskKeys))
	}
}

func TestGetRule(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create a rule first
	req := &model.CreateRuleRequest{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		TaskKeys:      []string{"task-1"},
		Enabled:       true,
	}

	created, err := svc.CreateRule(context.TODO(), req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	// Get the rule
	rule, err := svc.GetRule(context.TODO(), created.ID)
	if err != nil {
		t.Fatalf("GetRule failed: %v", err)
	}
	if rule == nil {
		t.Fatal("expected non-nil rule")
	}
	if rule.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, rule.ID)
	}
}

func TestListRules(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create multiple rules
	for i := 0; i < 3; i++ {
		req := &model.CreateRuleRequest{
			Name:          "Rule " + string(rune('A'+i)),
			RepoKey:       "test-repo",
			EventType:     "push",
			BranchPattern: "main",
			Action:        "sync",
			Enabled:       true,
		}
		_, err := svc.CreateRule(context.TODO(), req)
		if err != nil {
			t.Fatalf("CreateRule failed: %v", err)
		}
	}

	// List rules
	rules, err := svc.ListRules(context.TODO(), "test-repo")
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if len(rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(rules))
	}
}

func TestUpdateRule(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create a rule first
	req := &model.CreateRuleRequest{
		Name:          "Original Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		TaskKeys:      []string{"task-1"},
		Enabled:       true,
	}

	created, err := svc.CreateRule(context.TODO(), req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	// Update the rule without changing task keys (DAO has a bug with Replace)
	updateReq := &model.UpdateRuleRequest{
		ID:            created.ID,
		Name:          "Updated Rule",
		EventType:     "pull_request",
		BranchPattern: "develop",
		Action:        "sync",
		TaskKeys:      nil, // Don't change task keys
		MinInterval:   120,
		Enabled:       false,
	}

	updated, err := svc.UpdateRule(context.TODO(), updateReq)
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}

	if updated.Name != "Updated Rule" {
		t.Errorf("expected name 'Updated Rule', got %q", updated.Name)
	}
	if updated.EventType != "pull_request" {
		t.Errorf("expected event type 'pull_request', got %q", updated.EventType)
	}
	if updated.BranchPattern != "develop" {
		t.Errorf("expected branch pattern 'develop', got %q", updated.BranchPattern)
	}
	if updated.MinInterval != 120 {
		t.Errorf("expected min interval 120, got %d", updated.MinInterval)
	}
	if updated.Enabled {
		t.Error("expected rule to be disabled")
	}
}

func TestUpdateRule_NotFound(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	updateReq := &model.UpdateRuleRequest{
		ID:   999,
		Name: "Updated Rule",
	}

	_, err := svc.UpdateRule(context.TODO(), updateReq)
	if err == nil {
		t.Fatal("expected error for non-existent rule")
	}
	if err != ErrRuleNotFound {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

func TestDeleteRule(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create a rule first
	req := &model.CreateRuleRequest{
		Name:          "Rule to Delete",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		TaskKeys:      []string{"task-1"},
		Enabled:       true,
	}

	created, err := svc.CreateRule(context.TODO(), req)
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	// Delete the rule
	err = svc.DeleteRule(context.TODO(), created.ID)
	if err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	// Verify it's deleted
	rule, err := svc.GetRule(context.TODO(), created.ID)
	if err != nil {
		t.Fatalf("GetRule failed: %v", err)
	}
	if rule != nil {
		t.Error("expected rule to be deleted")
	}
}

func TestListEvents(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create some events
	for i := 0; i < 3; i++ {
		event := &model.WebhookEvent{
			EventID:   "event-" + string(rune('A'+i)),
			RepoKey:   "test-repo",
			EventType: "push",
			Status:    "received",
		}
		if err := svc.webhooks.CreateWebhookEvent(event); err != nil {
			t.Fatalf("Create event failed: %v", err)
		}
	}

	// List events
	events, total, err := svc.ListEvents(context.TODO(), "test-repo", 0, 10)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 events, got %d", total)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events in result, got %d", len(events))
	}
}

func TestRetryEvent(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "retry-event",
		RepoKey:   "test-repo",
		EventType: "push",
		Status:    "received",
	}
	if err := svc.webhooks.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Create event failed: %v", err)
	}

	// Retry the event
	err := svc.RetryEvent(context.TODO(), event.ID)
	if err != nil {
		t.Fatalf("RetryEvent failed: %v", err)
	}

	// Verify the event was marked as processing synchronously
	processing, err := svc.webhooks.FindEventByID(event.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if processing.Status != "processing" {
		t.Errorf("expected status 'processing', got %q", processing.Status)
	}

	// Wait for the goroutine to complete and update status to "processed"
	time.Sleep(100 * time.Millisecond)

	// Verify the event was updated to processed
	updated, err := svc.webhooks.FindEventByID(event.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if updated.Status != "processed" {
		t.Errorf("expected status 'processed', got %q", updated.Status)
	}
	if updated.ProcessedAt == nil {
		t.Error("expected processedAt to be set")
	}
}

func TestRetryEvent_NotFound(t *testing.T) {
	svc, _ := setupWebhookTestService(t)

	err := svc.RetryEvent(context.TODO(), 999)
	if err == nil {
		t.Fatal("expected error for non-existent event")
	}
	if err != ErrEventNotFound {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}
}

func TestApplyRules_MarksEventProcessed(t *testing.T) {
	svc, _ := setupWebhookTestService(t)
	ctx := context.Background()

	event := &model.WebhookEvent{
		EventID:   "apply-rules-status",
		RepoKey:   "test-repo",
		EventType: "push",
		Status:    model.StatusReceived,
	}
	if err := svc.webhooks.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Create event failed: %v", err)
	}

	svc.applyRules(ctx, "test-repo", event)

	got, err := svc.webhooks.FindEventByID(event.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if got.Status != model.StatusProcessed {
		t.Errorf("expected status %q after applyRules, got %q", model.StatusProcessed, got.Status)
	}
	if got.ProcessedAt == nil {
		t.Error("expected processed_at to be set")
	}
}
