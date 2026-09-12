package service

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupTaskServiceTestDB(t *testing.T) (*gorm.DB, *TaskService) {
	t.Helper()
	// Set up encryption key for tests
	key := "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256
	t.Setenv("ENCRYPTION_KEY", key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test db")

	err = db.AutoMigrate(&model.SyncTask{}, &model.SyncRun{}, &model.SyncRunStep{}, &model.Repo{})
	require.NoError(t, err, "failed to migrate test db")

	taskDAO := dao.NewSyncTaskDAO(db)
	runDAO := dao.NewSyncRunDAO(db)
	runStepDAO := dao.NewSyncRunStepDAO(db)
	repoDAO, err := dao.NewRepoDAO(db)
	require.NoError(t, err, "failed to create RepoDAO")

	svc := NewTaskService(taskDAO, runDAO, runStepDAO, repoDAO)

	return db, svc
}

func TestTaskService_ListTasks(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)
	ctx := context.Background()

	// Create tasks directly in database
	tasks := []*model.SyncTask{
		{Key: "task1", Name: "Task 1", WebhookToken: "token1"},
		{Key: "task2", Name: "Task 2", WebhookToken: "token2"},
		{Key: "task3", Name: "Task 3", WebhookToken: "token3"},
	}

	for _, task := range tasks {
		err := db.Create(task).Error
		require.NoError(t, err, "create failed")
	}

	// List all
	got, total, err := svc.ListTasks(ctx, "", 0, 50)
	require.NoError(t, err, "list failed")

	require.Equal(t, int64(3), total, "expected 3 tasks")
	require.Len(t, got, 3, "expected 3 tasks")
}

func TestTaskService_GetTask(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)
	ctx := context.Background()

	// Create task directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	// Get by key
	got, err := svc.GetTask(ctx, "test-task")
	require.NoError(t, err, "get failed")

	assert.Equal(t, "test-task", got.Key, "expected key 'test-task'")
	assert.Equal(t, "Test Task", got.Name, "expected name 'Test Task'")
}

func TestTaskService_GetTask_NotFound(t *testing.T) {
	_, svc := setupTaskServiceTestDB(t)
	ctx := context.Background()

	// Try to get a non-existent task
	got, err := svc.GetTask(ctx, "nonexistent")
	require.NoError(t, err, "unexpected error")

	assert.Nil(t, got, "expected nil when getting non-existent task")
}

func TestTaskService_CountTasksByStatus(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create tasks with different statuses
	tasks := []*model.SyncTask{
		{Key: "task1", Name: "Task 1", Enabled: true, LastStatus: "success", WebhookToken: "token1"},
		{Key: "task2", Name: "Task 2", Enabled: true, LastStatus: "success", WebhookToken: "token2"},
		{Key: "task3", Name: "Task 3", Enabled: true, LastStatus: "failed", WebhookToken: "token3"},
		{Key: "task4", Name: "Task 4", Enabled: false, WebhookToken: "token4"},
	}

	for _, task := range tasks {
		err := db.Create(task).Error
		require.NoError(t, err, "create failed")
	}

	// Count by status
	counts, err := svc.CountTasksByStatus()
	require.NoError(t, err, "count by status failed")

	assert.Equal(t, int64(4), counts["total"], "expected total 4")
	assert.Equal(t, int64(2), counts["success"], "expected success 2")
	assert.Equal(t, int64(1), counts["failed"], "expected failed 1")
}

func TestTaskService_CreateRun(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	// Create run
	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "create run failed")

	require.NotNil(t, run, "expected non-nil run")

	assert.Equal(t, "test-task", run.TaskKey, "expected task key 'test-task'")
	assert.Equal(t, "manual", run.TriggerSource, "expected trigger source 'manual'")
	assert.Equal(t, "running", run.Status, "expected status 'running'")
}

func TestTaskService_CompleteRun(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	// Create run
	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "create run failed")

	// Complete run
	run.Status = "success"
	run.Details = "Sync completed"

	err = svc.CompleteRun(run)
	require.NoError(t, err, "complete run failed")

	// Verify run was completed
	var got model.SyncRun
	err = db.First(&got, run.ID).Error
	require.NoError(t, err, "get run failed")

	assert.Equal(t, "success", got.Status, "expected status 'success'")
	assert.Equal(t, "Sync completed", got.Details, "expected details 'Sync completed'")
}

func TestTaskService_CreateRunStep(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task and run directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "create run failed")

	// Create run step
	step := &model.SyncRunStep{
		RunID:    run.ID,
		StepName: "fetch",
		Status:   "running",
	}

	err = svc.CreateRunStep(step)
	require.NoError(t, err, "create run step failed")

	assert.NotZero(t, step.ID, "expected step ID to be set after create")
}

func TestTaskService_UpdateRunStep(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task and run directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "create run failed")

	// Create run step
	step := &model.SyncRunStep{
		RunID:    run.ID,
		StepName: "fetch",
		Status:   "running",
	}

	err = svc.CreateRunStep(step)
	require.NoError(t, err, "create run step failed")

	// Update run step
	step.Status = "success"
	step.DurationMs = 500

	err = svc.UpdateRunStep(step)
	require.NoError(t, err, "update run step failed")

	// Verify step was updated
	var got model.SyncRunStep
	err = db.First(&got, step.ID).Error
	require.NoError(t, err, "get run step failed")

	assert.Equal(t, "success", got.Status, "expected status 'success'")
	assert.Equal(t, int64(500), got.DurationMs, "expected duration ms 500")
}

func TestTaskService_UpdateTaskLastRun(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	// Create run
	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "create run failed")

	// Complete run with end time
	now := time.Now()
	run.Status = "success"
	run.EndTime = &now

	err = svc.CompleteRun(run)
	require.NoError(t, err, "complete run failed")

	// Update task last run
	err = svc.UpdateTaskLastRun(task, run)
	require.NoError(t, err, "update task last run failed")

	// Verify task was updated
	var got model.SyncTask
	err = db.First(&got, task.ID).Error
	require.NoError(t, err, "get task failed")

	assert.Equal(t, "success", got.LastStatus, "expected last status 'success'")
	assert.NotNil(t, got.LastRunAt, "expected last run at to be set")
}

func TestTaskService_FindAllEnabledTasks(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create tasks with different enabled status
	// Use db.Create directly to avoid GORM default value issues with bool fields
	task1 := &model.SyncTask{Key: "task1", Name: "Task 1", Cron: "0 * * * *", WebhookToken: "token1"}
	task2 := &model.SyncTask{Key: "task2", Name: "Task 2", Cron: "*/5 * * * *", WebhookToken: "token2"}
	task3 := &model.SyncTask{Key: "task3", Name: "Task 3", Cron: "0 * * * *", WebhookToken: "token3"}
	task4 := &model.SyncTask{Key: "task4", Name: "Task 4", Cron: "", WebhookToken: "token4"}

	// Create tasks and explicitly set enabled status
	err := db.Create(task1).Error
	require.NoError(t, err, "create task1 failed")
	db.Model(task1).Update("enabled", true)

	err = db.Create(task2).Error
	require.NoError(t, err, "create task2 failed")
	db.Model(task2).Update("enabled", true)

	err = db.Create(task3).Error
	require.NoError(t, err, "create task3 failed")
	db.Model(task3).Update("enabled", false)

	err = db.Create(task4).Error
	require.NoError(t, err, "create task4 failed")
	db.Model(task4).Update("enabled", true)

	// Find all enabled tasks
	got, err := svc.FindAllEnabledTasks()
	require.NoError(t, err, "find all enabled failed")

	// Should return task1 and task2 (enabled with cron)
	// Should NOT return task3 (disabled) or task4 (no cron)
	require.Len(t, got, 2, "expected 2 enabled tasks")
}

func TestTaskService_FindTaskByKey(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create task directly in database
	task := &model.SyncTask{
		Key:  "test-task",
		Name: "Test Task",
	}

	err := db.Create(task).Error
	require.NoError(t, err, "create failed")

	// Find by key
	got, err := svc.FindTaskByKey("test-task")
	require.NoError(t, err, "find by key failed")

	assert.Equal(t, "test-task", got.Key, "expected key 'test-task'")
	assert.Equal(t, "Test Task", got.Name, "expected name 'Test Task'")
}

func TestTaskService_FindTaskByKey_NotFound(t *testing.T) {
	_, svc := setupTaskServiceTestDB(t)

	// Try to get a non-existent task
	got, err := svc.FindTaskByKey("nonexistent")
	require.NoError(t, err, "unexpected error")

	assert.Nil(t, got, "expected nil when getting non-existent task")
}

func TestTaskService_CleanupOldRuns(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

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
	count, err := svc.CleanupOldRuns(context.Background(), 24 * time.Hour)
	require.NoError(t, err, "cleanup failed")

	assert.Equal(t, int64(1), count, "expected count 1")
}

func TestTaskService_CleanupOldRunSteps(t *testing.T) {
	db, svc := setupTaskServiceTestDB(t)

	// Create a run
	run := &model.SyncRun{
		TaskKey:   "task1",
		Status:    "success",
		CreatedAt: time.Now(),
	}

	err := db.Create(run).Error
	require.NoError(t, err, "create run failed")

	// Create run steps with different ages
	oldStep := &model.SyncRunStep{
		RunID:     run.ID,
		StepName:  "fetch",
		Status:    "success",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	newStep := &model.SyncRunStep{
		RunID:     run.ID,
		StepName:  "push",
		Status:    "success",
		CreatedAt: time.Now(),
	}

	err = db.Create(oldStep).Error
	require.NoError(t, err, "create old step failed")

	err = db.Create(newStep).Error
	require.NoError(t, err, "create new step failed")

	// Cleanup older than 24 hours
	count, err := svc.CleanupOldRunSteps(context.Background(), 24 * time.Hour)
	require.NoError(t, err, "cleanup failed")

	assert.Equal(t, int64(1), count, "expected count 1")
}
