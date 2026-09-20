package converter

import (
	"testing"
	"time"

	"github.com/yi-nology/git-ferry-core/model"
)

func TestToTaskInfo(t *testing.T) {
	now := time.Now()
	task := &model.SyncTask{
		ID:           1,
		Key:          "test-key",
		Name:         "Test Task",
		SourceRepoKey: "source-repo",
		SourceBranch: "main",
		TargetRepoKey: "target-repo",
		TargetBranch: "main",
		SyncMode:     "mirror",
		Cron:         "0 * * * *",
		WebhookToken: "token123",
		Enabled:      true,
		GitTags:      true,
		GitForce:     false,
		GitPrune:     true,
		LastRunAt:    &now,
		LastStatus:   "success",
		CreatedAt:    now,
	}

	result := ToTaskInfo(task)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.Key != "test-key" {
		t.Errorf("Expected Key 'test-key', got '%s'", result.Key)
	}

	if result.Name != "Test Task" {
		t.Errorf("Expected Name 'Test Task', got '%s'", result.Name)
	}

	if result.SourceRepoKey != "source-repo" {
		t.Errorf("Expected SourceRepoKey 'source-repo', got '%s'", result.SourceRepoKey)
	}

	if result.SourceBranch != "main" {
		t.Errorf("Expected SourceBranch 'main', got '%s'", result.SourceBranch)
	}

	if result.TargetRepoKey != "target-repo" {
		t.Errorf("Expected TargetRepoKey 'target-repo', got '%s'", result.TargetRepoKey)
	}

	if result.TargetBranch != "main" {
		t.Errorf("Expected TargetBranch 'main', got '%s'", result.TargetBranch)
	}

	if result.SyncMode != "mirror" {
		t.Errorf("Expected SyncMode 'mirror', got '%s'", result.SyncMode)
	}

	if result.Cron != "0 * * * *" {
		t.Errorf("Expected Cron '0 * * * *', got '%s'", result.Cron)
	}

	// WebhookToken 不再下发(零消费点的 secret 字段),转换后必须为空
	if result.WebhookToken != "" {
		t.Errorf("Expected WebhookToken to be omitted, got '%s'", result.WebhookToken)
	}

	if !result.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if !result.GitTags {
		t.Error("Expected GitTags to be true")
	}

	if result.GitForce {
		t.Error("Expected GitForce to be false")
	}

	if !result.GitPrune {
		t.Error("Expected GitPrune to be true")
	}

	if result.LastStatus != "success" {
		t.Errorf("Expected LastStatus 'success', got '%s'", result.LastStatus)
	}
}

func TestToTaskInfoNil(t *testing.T) {
	result := ToTaskInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToTaskInfoList(t *testing.T) {
	tasks := []*model.SyncTask{
		{ID: 1, Key: "task1", Name: "Task 1"},
		{ID: 2, Key: "task2", Name: "Task 2"},
	}

	result := ToTaskInfoList(tasks)

	if len(result) != 2 {
		t.Fatalf("Expected 2 tasks, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first task ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second task ID 2, got %d", result[1].ID)
	}
}

func TestToTaskInfoListEmpty(t *testing.T) {
	result := ToTaskInfoList([]*model.SyncTask{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}

func TestToSyncRunInfo(t *testing.T) {
	now := time.Now()
	run := &model.SyncRun{
		ID:            1,
		TaskKey:       "test-task",
		TriggerSource: "manual",
		Status:        "success",
		StartTime:     now,
		EndTime:       &now,
		CommitRange:   "abc123..def456",
		Details:       "Sync completed",
		ErrorMessage:  "",
		CreatedAt:     now,
		DurationMs:    1000,
		ErrorType:     "",
		RetryTotal:    0,
	}

	result := ToSyncRunInfo(run)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.TaskKey != "test-task" {
		t.Errorf("Expected TaskKey 'test-task', got '%s'", result.TaskKey)
	}

	if result.TriggerSource != "manual" {
		t.Errorf("Expected TriggerSource 'manual', got '%s'", result.TriggerSource)
	}

	if result.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", result.Status)
	}

	if result.CommitRange != "abc123..def456" {
		t.Errorf("Expected CommitRange 'abc123..def456', got '%s'", result.CommitRange)
	}

	if result.Details != "Sync completed" {
		t.Errorf("Expected Details 'Sync completed', got '%s'", result.Details)
	}

	if result.DurationMs != 1000 {
		t.Errorf("Expected DurationMs 1000, got %d", result.DurationMs)
	}
}

func TestToSyncRunInfoNil(t *testing.T) {
	result := ToSyncRunInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToSyncRunInfoList(t *testing.T) {
	runs := []*model.SyncRun{
		{ID: 1, TaskKey: "task1", Status: "success"},
		{ID: 2, TaskKey: "task2", Status: "failed"},
	}

	result := ToSyncRunInfoList(runs)

	if len(result) != 2 {
		t.Fatalf("Expected 2 runs, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first run ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second run ID 2, got %d", result[1].ID)
	}
}

func TestToSyncRunStepInfo(t *testing.T) {
	now := time.Now()
	step := &model.SyncRunStep{
		ID:         1,
		StepName:   "fetch",
		Status:     "success",
		StartTime:  now,
		EndTime:    &now,
		DurationMs: 500,
		ErrorMsg:   "",
		ErrorType:  "",
		RetryCount: 0,
	}

	result := ToSyncRunStepInfo(step)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.StepName != "fetch" {
		t.Errorf("Expected StepName 'fetch', got '%s'", result.StepName)
	}

	if result.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", result.Status)
	}

	if result.DurationMs != 500 {
		t.Errorf("Expected DurationMs 500, got %d", result.DurationMs)
	}
}

func TestToSyncRunStepInfoNil(t *testing.T) {
	result := ToSyncRunStepInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToSyncRunStepInfoList(t *testing.T) {
	steps := []*model.SyncRunStep{
		{ID: 1, StepName: "fetch", Status: "success"},
		{ID: 2, StepName: "push", Status: "success"},
	}

	result := ToSyncRunStepInfoList(steps)

	if len(result) != 2 {
		t.Fatalf("Expected 2 steps, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first step ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second step ID 2, got %d", result[1].ID)
	}
}

func TestPageToOffset(t *testing.T) {
	tests := []struct {
		name           string
		page           int32
		pageSize       int32
		expectedOffset int
		expectedLimit  int
	}{
		{"normal", 1, 10, 0, 10},
		{"page 2", 2, 10, 10, 10},
		{"page 3", 3, 20, 40, 20},
		{"zero page", 0, 10, 0, 10},       // page defaults to 1
		{"negative page", -1, 10, 0, 10},   // page defaults to 1
		{"zero pageSize", 1, 0, 0, 50},     // pageSize defaults to 50
		{"negative pageSize", 1, -1, 0, 50}, // pageSize defaults to 50
		{"large pageSize", 1, 300, 0, 200}, // pageSize capped at 200
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, limit := PageToOffset(tt.page, tt.pageSize)
			if offset != tt.expectedOffset {
				t.Errorf("Expected offset %d, got %d", tt.expectedOffset, offset)
			}
			if limit != tt.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tt.expectedLimit, limit)
			}
		})
	}
}
