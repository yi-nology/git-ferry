package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupWebhookServiceTestDB(t *testing.T) (*gorm.DB, *WebhookService) {
	t.Helper()
	// Set up encryption key for tests
	key := "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256
	t.Setenv("ENCRYPTION_KEY", key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.WebhookRule{}, &model.WebhookRuleTask{}, &model.WebhookEvent{}, &model.Repo{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	ruleDAO := dao.NewWebhookRuleDAO(db)
	eventDAO := dao.NewWebhookEventDAO(db)
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("failed to create RepoDAO: %v", err)
	}

	svc := NewWebhookService(ruleDAO, eventDAO, repoDAO)

	return db, svc
}

func TestWebhookService_GetRule(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create rule directly in database
	rule := &model.WebhookRule{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		MinInterval:   60,
		Enabled:       true,
		Description:   "Test webhook rule",
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Get by ID
	got, err := svc.GetRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if got.Name != "Test Rule" {
		t.Errorf("expected name 'Test Rule', got '%s'", got.Name)
	}

	if got.RepoKey != "test-repo" {
		t.Errorf("expected repo key 'test-repo', got '%s'", got.RepoKey)
	}

	if got.EventType != "push" {
		t.Errorf("expected event type 'push', got '%s'", got.EventType)
	}

	if got.BranchPattern != "main" {
		t.Errorf("expected branch pattern 'main', got '%s'", got.BranchPattern)
	}

	if got.Action != "sync" {
		t.Errorf("expected action 'sync', got '%s'", got.Action)
	}

	if got.MinInterval != 60 {
		t.Errorf("expected min interval 60, got %d", got.MinInterval)
	}

	if !got.Enabled {
		t.Error("expected enabled to be true")
	}

	if got.Description != "Test webhook rule" {
		t.Errorf("expected description 'Test webhook rule', got '%s'", got.Description)
	}
}

func TestWebhookService_ListRules(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create rules directly in database
	rules := []*model.WebhookRule{
		{Name: "Rule 1", RepoKey: "repo1", EventType: "push"},
		{Name: "Rule 2", RepoKey: "repo1", EventType: "push"},
		{Name: "Rule 3", RepoKey: "repo1", EventType: "tag"},
	}

	for _, rule := range rules {
		if err := db.Create(rule).Error; err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	// List all for repo1
	got, err := svc.ListRules(ctx, "repo1")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(got))
	}
}

func TestWebhookService_ListRulesByRepoKey(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create rules with different repo keys
	rules := []*model.WebhookRule{
		{Name: "Rule 1", RepoKey: "repo1", EventType: "push"},
		{Name: "Rule 2", RepoKey: "repo1", EventType: "push"},
		{Name: "Rule 3", RepoKey: "repo2", EventType: "tag"},
	}

	for _, rule := range rules {
		if err := db.Create(rule).Error; err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	// List by repo key
	got, err := svc.ListRules(ctx, "repo1")
	if err != nil {
		t.Fatalf("list by repo key failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
}

func TestWebhookService_DeleteRule(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create rule directly in database
	rule := &model.WebhookRule{
		Name:    "Test Rule",
		RepoKey: "test-repo",
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Delete the rule
	if err := svc.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// Try to get the deleted rule
	got, err := svc.GetRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != nil {
		t.Error("expected nil when getting deleted rule")
	}
}

func TestWebhookService_GetRule_NotFound(t *testing.T) {
	_, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Try to get a non-existent rule
	got, err := svc.GetRule(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != nil {
		t.Error("expected nil when getting non-existent rule")
	}
}

func TestWebhookService_ListEvents(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create events directly in database
	events := []*model.WebhookEvent{
		{EventID: "evt-1", RepoKey: "repo1", EventType: "push", Status: "pending"},
		{EventID: "evt-2", RepoKey: "repo1", EventType: "push", Status: "processed"},
		{EventID: "evt-3", RepoKey: "repo1", EventType: "tag", Status: "failed"},
	}

	for _, event := range events {
		if err := db.Create(event).Error; err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	// List all for repo1
	got, total, err := svc.ListEvents(ctx, "repo1", 0, 50)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if total != 3 {
		t.Fatalf("expected 3 events, got %d", total)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
}

func TestWebhookService_ListEventsByRepoKey(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create events with different repo keys
	events := []*model.WebhookEvent{
		{EventID: "evt-1", RepoKey: "repo1", EventType: "push", Status: "pending"},
		{EventID: "evt-2", RepoKey: "repo1", EventType: "push", Status: "processed"},
		{EventID: "evt-3", RepoKey: "repo2", EventType: "tag", Status: "failed"},
	}

	for _, event := range events {
		if err := db.Create(event).Error; err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	// List by repo key
	got, total, err := svc.ListEvents(ctx, "repo1", 0, 50)
	if err != nil {
		t.Fatalf("list by repo key failed: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected 2 events, got %d", total)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
}

func TestWebhookService_MarkEventProcessing(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create event directly in database
	event := &model.WebhookEvent{
		EventID: "evt-123",
		RepoKey: "test-repo",
		Status:  "pending",
	}

	if err := db.Create(event).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Mark event as processing
	got, err := svc.MarkEventProcessing(ctx, event.ID)
	if err != nil {
		t.Fatalf("mark event processing failed: %v", err)
	}

	if got.Status != "processing" {
		t.Errorf("expected status 'processing', got '%s'", got.Status)
	}
}

func TestWebhookService_MarkEventProcessed(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)

	// Create event directly in database
	event := &model.WebhookEvent{
		EventID: "evt-123",
		RepoKey: "test-repo",
		Status:  "processing",
	}

	if err := db.Create(event).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Mark event as processed
	event.Status = "processed"
	if err := svc.MarkEventProcessed(event); err != nil {
		t.Fatalf("mark event processed failed: %v", err)
	}

	// Verify event was updated
	var got model.WebhookEvent
	if err := db.First(&got, event.ID).Error; err != nil {
		t.Fatalf("get event failed: %v", err)
	}

	if got.Status != "processed" {
		t.Errorf("expected status 'processed', got '%s'", got.Status)
	}
}

func TestWebhookService_FindEventByEventID(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)

	// Create event directly in database
	event := &model.WebhookEvent{
		EventID: "evt-123",
		RepoKey: "test-repo",
		Status:  "pending",
	}

	if err := db.Create(event).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Find by event ID
	got, err := svc.FindEventByEventID("evt-123")
	if err != nil {
		t.Fatalf("find by event ID failed: %v", err)
	}

	if got.EventID != "evt-123" {
		t.Errorf("expected event ID 'evt-123', got '%s'", got.EventID)
	}

	if got.RepoKey != "test-repo" {
		t.Errorf("expected repo key 'test-repo', got '%s'", got.RepoKey)
	}
}

func TestWebhookService_FindEventByID(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)

	// Create event directly in database
	event := &model.WebhookEvent{
		EventID: "evt-123",
		RepoKey: "test-repo",
		Status:  "pending",
	}

	if err := db.Create(event).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Find by ID
	got, err := svc.FindEventByID(event.ID)
	if err != nil {
		t.Fatalf("find by ID failed: %v", err)
	}

	if got.EventID != "evt-123" {
		t.Errorf("expected event ID 'evt-123', got '%s'", got.EventID)
	}

	if got.RepoKey != "test-repo" {
		t.Errorf("expected repo key 'test-repo', got '%s'", got.RepoKey)
	}
}

func TestWebhookService_CreateWebhookEvent(t *testing.T) {
	_, svc := setupWebhookServiceTestDB(t)

	// Create event
	event := &model.WebhookEvent{
		EventID: "evt-123",
		RepoKey: "test-repo",
		Status:  "pending",
	}

	if err := svc.CreateWebhookEvent(event); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if event.ID == 0 {
		t.Error("expected event ID to be set after create")
	}
}

func TestWebhookService_CleanupOldEvents(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)

	// Create events with different ages
	oldEvent := &model.WebhookEvent{
		EventID:   "evt-1",
		RepoKey:   "repo1",
		Status:    "processed",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	newEvent := &model.WebhookEvent{
		EventID:   "evt-2",
		RepoKey:   "repo1",
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := db.Create(oldEvent).Error; err != nil {
		t.Fatalf("create old event failed: %v", err)
	}

	if err := db.Create(newEvent).Error; err != nil {
		t.Fatalf("create new event failed: %v", err)
	}

	// Cleanup older than 24 hours
	count, err := svc.CleanupOldEvents(context.Background(), 24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Verify only new event remains
	got, err := svc.FindEventByID(newEvent.ID)
	if err != nil {
		t.Fatalf("find event failed: %v", err)
	}

	if got == nil {
		t.Error("expected new event to still exist")
	}
}

func TestWebhookService_ApplyRules(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create a rule
	rule := &model.WebhookRule{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		Enabled:       true,
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	// Create task keys for the rule
	taskKey := &model.WebhookRuleTask{
		RuleID:  rule.ID,
		TaskKey: "task1",
	}

	if err := db.Create(taskKey).Error; err != nil {
		t.Fatalf("create task key failed: %v", err)
	}

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Branch:    "main",
		Status:    "pending",
	}

	// Track task executions
	var executedTasks []string
	runTaskFn := func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error {
		executedTasks = append(executedTasks, taskKey)
		return nil
	}

	// Apply rules
	lastTriggerTime := &sync.Map{}
	svc.ApplyRules(ctx, "test-repo", event, lastTriggerTime, runTaskFn, nil)

	// Verify task was executed
	if len(executedTasks) != 1 {
		t.Fatalf("expected 1 task execution, got %d", len(executedTasks))
	}

	if executedTasks[0] != "task1" {
		t.Errorf("expected task key 'task1', got '%s'", executedTasks[0])
	}
}

func TestWebhookService_ApplyRules_DisabledRule(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create a rule
	rule := &model.WebhookRule{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	// Explicitly disable the rule
	if err := db.Model(rule).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable rule failed: %v", err)
	}

	// Create task keys for the rule
	taskKey := &model.WebhookRuleTask{
		RuleID:  rule.ID,
		TaskKey: "task1",
	}

	if err := db.Create(taskKey).Error; err != nil {
		t.Fatalf("create task key failed: %v", err)
	}

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Branch:    "main",
		Status:    "pending",
	}

	// Track task executions
	var executedTasks []string
	runTaskFn := func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error {
		executedTasks = append(executedTasks, taskKey)
		return nil
	}

	// Apply rules
	lastTriggerTime := &sync.Map{}
	svc.ApplyRules(ctx, "test-repo", event, lastTriggerTime, runTaskFn, nil)

	// Verify no tasks were executed
	if len(executedTasks) != 0 {
		t.Fatalf("expected 0 task executions, got %d", len(executedTasks))
	}
}

func TestWebhookService_ApplyRules_WrongEventType(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create a rule for push events
	rule := &model.WebhookRule{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		Enabled:       true,
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	// Create task keys for the rule
	taskKey := &model.WebhookRuleTask{
		RuleID:  rule.ID,
		TaskKey: "task1",
	}

	if err := db.Create(taskKey).Error; err != nil {
		t.Fatalf("create task key failed: %v", err)
	}

	// Create an event with different type
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "tag",
		Branch:    "main",
		Status:    "pending",
	}

	// Track task executions
	var executedTasks []string
	runTaskFn := func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error {
		executedTasks = append(executedTasks, taskKey)
		return nil
	}

	// Apply rules
	lastTriggerTime := &sync.Map{}
	svc.ApplyRules(ctx, "test-repo", event, lastTriggerTime, runTaskFn, nil)

	// Verify no tasks were executed
	if len(executedTasks) != 0 {
		t.Fatalf("expected 0 task executions, got %d", len(executedTasks))
	}
}

func TestWebhookService_ApplyRules_WrongBranch(t *testing.T) {
	db, svc := setupWebhookServiceTestDB(t)
	ctx := context.Background()

	// Create a rule for main branch
	rule := &model.WebhookRule{
		Name:          "Test Rule",
		RepoKey:       "test-repo",
		EventType:     "push",
		BranchPattern: "main",
		Action:        "sync",
		Enabled:       true,
	}

	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	// Create task keys for the rule
	taskKey := &model.WebhookRuleTask{
		RuleID:  rule.ID,
		TaskKey: "task1",
	}

	if err := db.Create(taskKey).Error; err != nil {
		t.Fatalf("create task key failed: %v", err)
	}

	// Create an event with different branch
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Branch:    "develop",
		Status:    "pending",
	}

	// Track task executions
	var executedTasks []string
	runTaskFn := func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error {
		executedTasks = append(executedTasks, taskKey)
		return nil
	}

	// Apply rules
	lastTriggerTime := &sync.Map{}
	svc.ApplyRules(ctx, "test-repo", event, lastTriggerTime, runTaskFn, nil)

	// Verify no tasks were executed
	if len(executedTasks) != 0 {
		t.Fatalf("expected 0 task executions, got %d", len(executedTasks))
	}
}
