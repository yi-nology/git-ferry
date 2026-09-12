package service

import (
	"context"
	"testing"

	"github.com/robfig/cron/v3"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.SyncTask{}, &model.SyncRun{}, &model.SyncRunStep{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}

func setupTaskTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	db := setupTaskTestDB(t)
	taskDAO := dao.NewSyncTaskDAO(db)
	runDAO := dao.NewSyncRunDAO(db)

	runStepDAO := dao.NewSyncRunStepDAO(db)
	taskService := NewTaskService(taskDAO, runDAO, runStepDAO, nil)

	svc := &Service{
		tasks:          taskService,
		cron:           cron.New(),
		cronEntryIDs:   make(map[string]cron.EntryID),
		config: &model.Config{
			Sync: model.SyncConfig{
				DefaultTimeout: 300,
				RetryCount:     3,
			},
		},
	}
	return svc, db
}

func TestCreateTask(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	req := &model.CreateTaskRequest{
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
		SyncMode:      "single",
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if task.Name != "Test Task" {
		t.Errorf("expected name 'Test Task', got %q", task.Name)
	}
	if task.SourceRepoKey != "source-repo" {
		t.Errorf("expected source repo key 'source-repo', got %q", task.SourceRepoKey)
	}
	if task.TargetRepoKey != "target-repo" {
		t.Errorf("expected target repo key 'target-repo', got %q", task.TargetRepoKey)
	}
	if task.SourceBranch != "main" {
		t.Errorf("expected source branch 'main', got %q", task.SourceBranch)
	}
	if task.TargetBranch != "main" {
		t.Errorf("expected target branch 'main', got %q", task.TargetBranch)
	}
	if task.SyncMode != "single" {
		t.Errorf("expected sync mode 'single', got %q", task.SyncMode)
	}
	if !task.Enabled {
		t.Error("expected task to be enabled")
	}
	if task.Key == "" {
		t.Error("expected non-empty key")
	}
	if task.WebhookToken == "" {
		t.Error("expected non-empty webhook token")
	}
}

func TestCreateTask_WithCron(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	req := &model.CreateTaskRequest{
		Name:          "Cron Task",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
		Cron:          "*/5 * * * *",
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if task.Cron != "*/5 * * * *" {
		t.Errorf("expected cron '*/5 * * * *', got %q", task.Cron)
	}

	// Verify cron job was added
	if _, exists := svc.cronEntryIDs[task.Key]; !exists {
		t.Error("expected cron job to be added")
	}
}

func TestGetTask(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create a task first
	req := &model.CreateTaskRequest{
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
	}

	created, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Get the task
	task, err := svc.GetTask(ctx, created.Key)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Key != created.Key {
		t.Errorf("expected key %q, got %q", created.Key, task.Key)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	task, err := svc.GetTask(ctx, "non-existent-key")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task != nil {
		t.Error("expected nil task for non-existent key")
	}
}

func TestListTasks(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create multiple tasks
	for i := 0; i < 3; i++ {
		req := &model.CreateTaskRequest{
			Name:          "Task " + string(rune('A'+i)),
			SourceRepoKey: "source-repo",
			SourceBranch:  "main",
			TargetRepoKey: "target-repo",
			TargetBranch:  "main",
		}
		_, err := svc.CreateTask(ctx, req)
		if err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}
	}

	// List all tasks
	tasks, total, err := svc.ListTasks(ctx, "", 0, 10)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 tasks, got %d", total)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks in result, got %d", len(tasks))
	}
}

func TestUpdateTask(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create a task first
	req := &model.CreateTaskRequest{
		Name:          "Original Task",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
	}

	created, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Update the task
	updateReq := &model.UpdateTaskRequest{
		Key:          created.Key,
		Name:         "Updated Task",
		SourceBranch: "develop",
		TargetBranch: "staging",
	}

	updated, err := svc.UpdateTask(ctx, updateReq)
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	if updated.Name != "Updated Task" {
		t.Errorf("expected name 'Updated Task', got %q", updated.Name)
	}
	if updated.SourceBranch != "develop" {
		t.Errorf("expected source branch 'develop', got %q", updated.SourceBranch)
	}
	if updated.TargetBranch != "staging" {
		t.Errorf("expected target branch 'staging', got %q", updated.TargetBranch)
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	updateReq := &model.UpdateTaskRequest{
		Key:  "non-existent-key",
		Name: "Updated Task",
	}

	_, err := svc.UpdateTask(ctx, updateReq)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if err != ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestDeleteTask(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create a task first
	req := &model.CreateTaskRequest{
		Name:          "Task to Delete",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
		Cron:          "*/5 * * * *",
	}

	created, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Verify cron job exists
	if _, exists := svc.cronEntryIDs[created.Key]; !exists {
		t.Error("expected cron job to exist before deletion")
	}

	// Delete the task
	err = svc.DeleteTask(ctx, created.Key)
	if err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify task is deleted
	task, err := svc.GetTask(ctx, created.Key)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task != nil {
		t.Error("expected task to be deleted")
	}

	// Verify cron job is removed
	if _, exists := svc.cronEntryIDs[created.Key]; exists {
		t.Error("expected cron job to be removed after deletion")
	}
}

func TestListHistory(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create a task first
	req := &model.CreateTaskRequest{
		Name:          "Task with History",
		SourceRepoKey: "source-repo",
		SourceBranch:  "main",
		TargetRepoKey: "target-repo",
		TargetBranch:  "main",
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Create some run history
	for i := 0; i < 3; i++ {
		if _, err := svc.tasks.CreateRun(task, "manual", nil); err != nil {
			t.Fatalf("Create run failed: %v", err)
		}
	}

	// List history
	runs, total, err := svc.ListHistory(ctx, task.Key, 0, 10)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 runs, got %d", total)
	}
	if len(runs) != 3 {
		t.Errorf("expected 3 runs in result, got %d", len(runs))
	}
}

func TestDeleteHistory(t *testing.T) {
	svc, _ := setupTaskTestService(t)
	ctx := context.Background()

	// Create a task and a run
	task, err := svc.CreateTask(ctx, &model.CreateTaskRequest{
		Name: "test-task", SourceRepoKey: "s", SourceBranch: "main",
		TargetRepoKey: "t", TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	run, err := svc.tasks.CreateRun(task, "manual", nil)
	if err != nil {
		t.Fatalf("Create run failed: %v", err)
	}

	// Delete the history
	err = svc.DeleteHistory(ctx, run.ID)
	if err != nil {
		t.Fatalf("DeleteHistory failed: %v", err)
	}

	// Verify it's deleted
	runs, total, err := svc.ListHistory(ctx, "test-task", 0, 10)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 runs after deletion, got %d", total)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs in result, got %d", len(runs))
	}
}

func TestPreviewSync_WithRepoDAO(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	db := setupTaskTestDB(t)
	// Migrate Repo model as well
	if err := db.AutoMigrate(&model.Repo{}); err != nil {
		t.Fatalf("failed to migrate repo: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	runDAO := dao.NewSyncRunDAO(db)
	runStepDAO := dao.NewSyncRunStepDAO(db)
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("failed to create RepoDAO: %v", err)
	}

	taskService := NewTaskService(taskDAO, runDAO, runStepDAO, repoDAO)

	svc := &Service{
		tasks:         taskService,
		cron:          cron.New(),
		cronEntryIDs:  make(map[string]cron.EntryID),
	}

	ctx := context.Background()

	// Create repos
	if err := repoDAO.Create(&model.Repo{
		Key:      "source-repo",
		Name:     "Source",
		CloneURL: "https://github.com/source/repo.git",
		Status:   "active",
	}); err != nil {
		t.Fatalf("Create source repo failed: %v", err)
	}
	if err := repoDAO.Create(&model.Repo{
		Key:      "target-repo",
		Name:     "Target",
		CloneURL: "https://github.com/target/repo.git",
		Status:   "active",
	}); err != nil {
		t.Fatalf("Create target repo failed: %v", err)
	}

	req := &model.PreviewSyncRequest{
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
	}

	result, err := svc.PreviewSync(ctx, req)
	if err != nil {
		t.Fatalf("PreviewSync failed: %v", err)
	}
	if !result.CanSync {
		t.Error("expected CanSync to be true")
	}
	if !result.SourceExists {
		t.Error("expected SourceExists to be true")
	}
	if !result.TargetExists {
		t.Error("expected TargetExists to be true")
	}
}

func TestPreviewSync_MissingRepo(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	db := setupTaskTestDB(t)
	if err := db.AutoMigrate(&model.Repo{}); err != nil {
		t.Fatalf("failed to migrate repo: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	runDAO := dao.NewSyncRunDAO(db)
	runStepDAO := dao.NewSyncRunStepDAO(db)
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("failed to create RepoDAO: %v", err)
	}

	taskService := NewTaskService(taskDAO, runDAO, runStepDAO, repoDAO)

	svc := &Service{
		tasks:         taskService,
		cron:          cron.New(),
		cronEntryIDs:  make(map[string]cron.EntryID),
	}

	ctx := context.Background()

	req := &model.PreviewSyncRequest{
		SourceRepoKey: "missing-source",
		TargetRepoKey: "missing-target",
	}

	result, err := svc.PreviewSync(ctx, req)
	if err != nil {
		t.Fatalf("PreviewSync failed: %v", err)
	}
	if result.CanSync {
		t.Error("expected CanSync to be false when repos are missing")
	}
}

func TestPreviewSync(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	db := setupTaskTestDB(t)
	if err := db.AutoMigrate(&model.Repo{}); err != nil {
		t.Fatalf("migrate repo: %v", err)
	}
	repoDAO, err := dao.NewRepoDAO(db)
	if err != nil {
		t.Fatalf("NewRepoDAO: %v", err)
	}
	if err := repoDAO.Create(&model.Repo{Key: "src", Name: "src", Status: model.RepoStatusActive}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	ts := NewTaskService(dao.NewSyncTaskDAO(db), dao.NewSyncRunDAO(db), dao.NewSyncRunStepDAO(db), repoDAO)
	ctx := context.Background()

	// 目标缺失
	r1, err := ts.PreviewSync(ctx, &model.PreviewSyncRequest{SourceRepoKey: "src", TargetRepoKey: "missing"})
	if err != nil || r1.CanSync || !r1.SourceExists || r1.TargetExists {
		t.Errorf("case1 unexpected: %+v err=%v", r1, err)
	}
	// 双侧存在
	r2, err := ts.PreviewSync(ctx, &model.PreviewSyncRequest{SourceRepoKey: "src", TargetRepoKey: "src"})
	if err != nil || !r2.CanSync || !r2.SourceExists || !r2.TargetExists {
		t.Errorf("case2 unexpected: %+v err=%v", r2, err)
	}
}
