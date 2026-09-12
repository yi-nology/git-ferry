package dao

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupSyncRunTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test db")

	err = db.AutoMigrate(&model.SyncRun{}, &model.SyncRunStep{})
	require.NoError(t, err, "failed to migrate test db")

	return db
}

func TestSyncRunDAO_CreateAndFindByTaskKey(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create a run
	run := &model.SyncRun{
		TaskKey:       "test-task",
		TriggerSource: "manual",
		Status:        "running",
		StartTime:     time.Now(),
	}

	err := d.Create(run)
	require.NoError(t, err, "create failed")

	assert.NotZero(t, run.ID, "expected run ID to be set after create")

	// Find by task key
	got, total, err := d.FindByTaskKey("test-task", DefaultPagination(0, 50))
	require.NoError(t, err, "find by task key failed")

	require.Equal(t, int64(1), total, "expected 1 run")
	require.Len(t, got, 1, "expected 1 run")

	assert.Equal(t, "test-task", got[0].TaskKey, "expected task key 'test-task'")
	assert.Equal(t, "manual", got[0].TriggerSource, "expected trigger source 'manual'")
	assert.Equal(t, "running", got[0].Status, "expected status 'running'")
}

func TestSyncRunDAO_FindRecent(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create multiple runs
	runs := []*model.SyncRun{
		{TaskKey: "task1", TriggerSource: "manual", Status: "success"},
		{TaskKey: "task2", TriggerSource: "cron", Status: "failed"},
		{TaskKey: "task3", TriggerSource: "webhook", Status: "running"},
	}

	for _, run := range runs {
		err := d.Create(run)
		require.NoError(t, err, "create failed")
	}

	// Find recent
	got, err := d.FindRecent(DefaultPagination(0, 50))
	require.NoError(t, err, "find recent failed")

	require.Len(t, got, 3, "expected 3 runs")
}

func TestSyncRunDAO_Update(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create a run
	run := &model.SyncRun{
		TaskKey:       "test-task",
		TriggerSource: "manual",
		Status:        "running",
		StartTime:     time.Now(),
	}

	err := d.Create(run)
	require.NoError(t, err, "create failed")

	// Update the run
	now := time.Now()
	run.Status = "success"
	run.EndTime = &now
	run.Details = "Sync completed"
	run.DurationMs = 1000

	err = d.Update(run)
	require.NoError(t, err, "update failed")

	// Find by task key to verify update
	got, total, err := d.FindByTaskKey("test-task", DefaultPagination(0, 50))
	require.NoError(t, err, "find by task key failed")

	require.Equal(t, int64(1), total, "expected 1 run")

	assert.Equal(t, "success", got[0].Status, "expected status 'success'")
	assert.Equal(t, "Sync completed", got[0].Details, "expected details 'Sync completed'")
	assert.Equal(t, int64(1000), got[0].DurationMs, "expected duration ms 1000")
}

func TestSyncRunDAO_Delete(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create a run
	run := &model.SyncRun{
		TaskKey:       "test-task",
		TriggerSource: "manual",
		Status:        "running",
	}

	err := d.Create(run)
	require.NoError(t, err, "create failed")

	// Delete the run
	err = d.Delete(run.ID)
	require.NoError(t, err, "delete failed")

	// Find by task key should return empty
	got, total, err := d.FindByTaskKey("test-task", DefaultPagination(0, 50))
	require.NoError(t, err, "find by task key failed")

	require.Equal(t, int64(0), total, "expected 0 runs")
	require.Len(t, got, 0, "expected 0 runs")
}

func TestSyncRunDAO_CountByTaskKey(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create runs with different task keys
	runs := []*model.SyncRun{
		{TaskKey: "task1", Status: "success"},
		{TaskKey: "task1", Status: "failed"},
		{TaskKey: "task2", Status: "running"},
	}

	for _, run := range runs {
		err := d.Create(run)
		require.NoError(t, err, "create failed")
	}

	// Count by task key
	count, err := d.CountByTaskKey("task1")
	require.NoError(t, err, "count by task key failed")

	assert.Equal(t, int64(2), count, "expected count 2")
}

func TestSyncRunDAO_CleanupOlderThan(t *testing.T) {
	db := setupSyncRunTestDB(t)
	d := NewSyncRunDAO(db)

	// Create runs with different ages
	oldRun := &model.SyncRun{
		TaskKey:   "task1",
		Status:    "success",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	newRun := &model.SyncRun{
		TaskKey:   "task2",
		Status:    "success",
		CreatedAt: time.Now(),
	}

	err := db.Create(oldRun).Error
	require.NoError(t, err, "create old run failed")

	err = db.Create(newRun).Error
	require.NoError(t, err, "create new run failed")

	// Cleanup older than 24 hours
	count, err := d.CleanupOlderThan(context.Background(), 24 * time.Hour)
	require.NoError(t, err, "cleanup failed")

	assert.Equal(t, int64(1), count, "expected count 1")

	// Verify only new run remains
	got, err := d.FindRecent(DefaultPagination(0, 50))
	require.NoError(t, err, "find recent failed")

	require.Len(t, got, 1, "expected 1 run")

	assert.Equal(t, "task2", got[0].TaskKey, "expected task key 'task2'")
}

func TestSyncRunDAO_Fields(t *testing.T) {
	now := time.Now()
	run := model.SyncRun{
		ID:             1,
		TaskKey:        "test-task",
		TriggerSource:  "manual",
		Status:         "success",
		StartTime:      now,
		EndTime:        &now,
		CommitRange:    "abc123..def456",
		Details:        "Sync completed",
		ErrorMessage:   "",
		ErrorType:      "",
		DurationMs:     1000,
		RetryTotal:     0,
		WebhookEventID: nil,
		CreatedAt:      now,
	}

	assert.Equal(t, uint(1), run.ID, "expected ID 1")
	assert.Equal(t, "test-task", run.TaskKey, "expected task key 'test-task'")
	assert.Equal(t, "manual", run.TriggerSource, "expected trigger source 'manual'")
	assert.Equal(t, "success", run.Status, "expected status 'success'")
	assert.Equal(t, "abc123..def456", run.CommitRange, "expected commit range 'abc123..def456'")
	assert.Equal(t, "Sync completed", run.Details, "expected details 'Sync completed'")
	assert.Equal(t, "", run.ErrorMessage, "expected empty error message")
	assert.Equal(t, "", run.ErrorType, "expected empty error type")
	assert.Equal(t, int64(1000), run.DurationMs, "expected duration ms 1000")
	assert.Equal(t, 0, run.RetryTotal, "expected retry total 0")
	assert.Nil(t, run.WebhookEventID, "expected webhook event ID to be nil")
}
