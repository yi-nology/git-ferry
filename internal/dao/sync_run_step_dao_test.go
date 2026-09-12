package dao

import (
	"context"
	"testing"
	"time"

	"github.com/yi-nology/git-sync-service/sync/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSyncRunStepTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.SyncRunStep{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}

func TestSyncRunStepDAO_CreateAndFindByRunID(t *testing.T) {
	db := setupSyncRunStepTestDB(t)
	d := NewSyncRunStepDAO(db)

	// Create steps
	steps := []*model.SyncRunStep{
		{RunID: 1, StepName: "fetch", Status: "success", DurationMs: 100},
		{RunID: 1, StepName: "push", Status: "success", DurationMs: 200},
		{RunID: 2, StepName: "fetch", Status: "failed", DurationMs: 50},
	}

	for _, step := range steps {
		if err := d.Create(step); err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	if steps[0].ID == 0 {
		t.Error("expected step ID to be set after create")
	}

	// Find by run ID
	got, err := d.FindByRunID(1)
	if err != nil {
		t.Fatalf("find by run ID failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(got))
	}
}

func TestSyncRunStepDAO_Update(t *testing.T) {
	db := setupSyncRunStepTestDB(t)
	d := NewSyncRunStepDAO(db)

	// Create a step
	step := &model.SyncRunStep{
		RunID:    1,
		StepName: "fetch",
		Status:   "running",
	}

	if err := d.Create(step); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Update the step
	now := time.Now()
	step.Status = "success"
	step.EndTime = &now
	step.DurationMs = 500

	if err := d.Update(step); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Find by run ID to verify update
	got, err := d.FindByRunID(1)
	if err != nil {
		t.Fatalf("find by run ID failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got))
	}

	if got[0].Status != "success" {
		t.Errorf("expected status 'success', got '%s'", got[0].Status)
	}

	if got[0].DurationMs != 500 {
		t.Errorf("expected duration ms 500, got %d", got[0].DurationMs)
	}
}

func TestSyncRunStepDAO_FindByRunID_NotFound(t *testing.T) {
	db := setupSyncRunStepTestDB(t)
	d := NewSyncRunStepDAO(db)

	// Try to get steps for a non-existent run
	got, err := d.FindByRunID(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected 0 steps, got %d", len(got))
	}
}

func TestSyncRunStepDAO_CleanupOlderThan(t *testing.T) {
	db := setupSyncRunStepTestDB(t)
	d := NewSyncRunStepDAO(db)

	// Create steps with different ages
	oldStep := &model.SyncRunStep{
		RunID:     1,
		StepName:  "fetch",
		Status:    "success",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	newStep := &model.SyncRunStep{
		RunID:     2,
		StepName:  "fetch",
		Status:    "success",
		CreatedAt: time.Now(),
	}

	if err := db.Create(oldStep).Error; err != nil {
		t.Fatalf("create old step failed: %v", err)
	}

	if err := db.Create(newStep).Error; err != nil {
		t.Fatalf("create new step failed: %v", err)
	}

	// Cleanup older than 24 hours
	count, err := d.CleanupOlderThan(context.Background(), 24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Verify only new step remains
	got, err := d.FindByRunID(2)
	if err != nil {
		t.Fatalf("find by run ID failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got))
	}

	if got[0].StepName != "fetch" {
		t.Errorf("expected step name 'fetch', got '%s'", got[0].StepName)
	}
}

func TestSyncRunStepDAO_Fields(t *testing.T) {
	now := time.Now()
	step := model.SyncRunStep{
		ID:          1,
		RunID:       1,
		StepName:    "fetch",
		Status:      "success",
		StartTime:   now,
		EndTime:     &now,
		DurationMs:  500,
		ErrorMsg:    "",
		ErrorType:   "",
		RetryCount:  0,
		CreatedAt:   now,
	}

	if step.ID != 1 {
		t.Errorf("expected ID 1, got %d", step.ID)
	}

	if step.RunID != 1 {
		t.Errorf("expected run ID 1, got %d", step.RunID)
	}

	if step.StepName != "fetch" {
		t.Errorf("expected step name 'fetch', got '%s'", step.StepName)
	}

	if step.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", step.Status)
	}

	if step.DurationMs != 500 {
		t.Errorf("expected duration ms 500, got %d", step.DurationMs)
	}

	if step.ErrorMsg != "" {
		t.Errorf("expected empty error message, got '%s'", step.ErrorMsg)
	}

	if step.ErrorType != "" {
		t.Errorf("expected empty error type, got '%s'", step.ErrorType)
	}

	if step.RetryCount != 0 {
		t.Errorf("expected retry count 0, got %d", step.RetryCount)
	}
}
