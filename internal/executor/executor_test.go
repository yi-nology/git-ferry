package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-platform-sdk/gitbackend"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// mockService implements Service interface for testing.
type mockService struct {
	runs        []*model.SyncRun
	tasks       []*model.SyncTask
	repos       map[string]*model.Repo
	createErr   error
	completeErr error
	updateErr   error
	repoErr     error
	tempDir     string
	config      *model.Config
}

func (m *mockService) CreateRun(task *model.SyncTask, trigger string, webhookEventID *uint) (*model.SyncRun, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	run := &model.SyncRun{
		TaskKey:        task.Key,
		TriggerSource:  trigger,
		Status:         "running",
		StartTime:      time.Now(),
		WebhookEventID: webhookEventID,
	}
	run.ID = uint(len(m.runs) + 1)
	m.runs = append(m.runs, run)
	return run, nil
}

func (m *mockService) CreateRunStep(step *model.SyncRunStep) error {
	return nil
}

func (m *mockService) UpdateRunStep(step *model.SyncRunStep) error {
	return nil
}

func (m *mockService) CompleteRun(run *model.SyncRun) error {
	if m.completeErr != nil {
		return m.completeErr
	}
	for i, r := range m.runs {
		if r.ID == run.ID {
			m.runs[i] = run
			break
		}
	}
	return nil
}

func (m *mockService) UpdateTaskLastRun(task *model.SyncTask, run *model.SyncRun) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	task.LastRunAt = run.EndTime
	task.LastStatus = run.Status
	return nil
}

func (m *mockService) CompleteRunWithTaskUpdate(run *model.SyncRun, task *model.SyncTask) error {
	if err := m.CompleteRun(run); err != nil {
		return err
	}
	return m.UpdateTaskLastRun(task, run)
}

func (m *mockService) GetRepoByKey(key string) (*model.Repo, error) {
	if m.repoErr != nil {
		return nil, m.repoErr
	}
	repo, ok := m.repos[key]
	if !ok {
		return nil, nil
	}
	return repo, nil
}

func (m *mockService) GetTempDir(taskKey string) string {
	return m.tempDir
}

func (m *mockService) GetConfig() *model.Config {
	return m.config
}

func (m *mockService) GetPlatformByID(_ context.Context, id uint) (*model.Platform, error) {
	return &model.Platform{
		ID:          id,
		Key:         "test-platform",
		Name:        "Test Platform",
		Type:        "github",
		AccessToken: "test-token",
	}, nil
}

func newMockService() *mockService {
	return &mockService{
		runs:  make([]*model.SyncRun, 0),
		tasks: make([]*model.SyncTask, 0),
		repos: map[string]*model.Repo{
			"source-repo": {
				Key:         "source-repo",
				Name:        "Source Repo",
				CloneURL:    "https://github.com/source/repo.git",
				AccessToken: "source-token",
			},
			"target-repo": {
				Key:         "target-repo",
				Name:        "Target Repo",
				CloneURL:    "https://github.com/target/repo.git",
				AccessToken: "target-token",
			},
		},
		tempDir: "/tmp/git-sync-test",
		config: &model.Config{
			Sync: model.SyncConfig{
				DefaultTimeout: 300,
				RetryCount:     3,
			},
		},
	}
}

func TestNewExecutor(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")
	require.NotNil(t, exec, "expected non-nil executor")
	assert.Equal(t, svc, exec.service, "expected service to be set")
	assert.NotNil(t, exec.backend, "expected backend to be set")
}

func TestExecute_SourceRepoNotFound(t *testing.T) {
	svc := newMockService()
	svc.repos = map[string]*model.Repo{} // Empty repos

	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "missing-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	ctx := context.Background()
	run, err := exec.Execute(ctx, task, "manual", nil)
	require.Error(t, err, "expected error for missing source repo")
	require.NotNil(t, run, "expected non-nil run even on error")
	assert.Equal(t, "failed", run.Status, "expected status 'failed'")
}

func TestExecute_TargetRepoNotFound(t *testing.T) {
	svc := newMockService()
	svc.repos = map[string]*model.Repo{
		"source-repo": {
			Key:      "source-repo",
			CloneURL: "https://github.com/source/repo.git",
		},
	}

	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "missing-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	ctx := context.Background()
	run, err := exec.Execute(ctx, task, "manual", nil)
	require.Error(t, err, "expected error for missing target repo")
	require.NotNil(t, run, "expected non-nil run even on error")
	assert.Equal(t, "failed", run.Status, "expected status 'failed'")
}

func TestExecute_RunCreated(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	ctx := context.Background()
	_, _ = exec.Execute(ctx, task, "manual", nil)

	// Verify run was created
	require.NotEmpty(t, svc.runs, "expected run to be created")

	run := svc.runs[0]
	assert.Equal(t, "test-task", run.TaskKey, "expected task key 'test-task'")
	assert.Equal(t, "manual", run.TriggerSource, "expected trigger 'manual'")
	assert.False(t, run.StartTime.IsZero(), "expected start time to be set")
}

func TestExecute_CreateRunError(t *testing.T) {
	svc := newMockService()
	svc.createErr = fmt.Errorf("database error")

	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	ctx := context.Background()
	_, err = exec.Execute(ctx, task, "manual", nil)
	require.Error(t, err, "expected error when run creation fails")
}

func TestAuthConfig_WithToken(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{
		AccessToken: "test-token",
	}

	config := exec.authConfig(context.Background(), repo, nil)
	assert.Equal(t, gitbackend.AuthHTTPBasic, config.Type, "expected HTTP Basic auth")
	assert.Equal(t, "test-token", config.Password, "expected password 'test-token'")
	assert.NotEmpty(t, config.Username, "expected non-empty placeholder username for HTTP Basic")
}

func TestAuthConfig_WithoutToken(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{}

	config := exec.authConfig(context.Background(), repo, nil)
	assert.Equal(t, gitbackend.AuthNone, config.Type, "expected auth type none")
}

func TestTimePtr(t *testing.T) {
	now := time.Now()
	ptr := timePtr(now)
	require.NotNil(t, ptr, "expected non-nil time pointer")
	assert.Equal(t, now, *ptr, "expected time pointer value")
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, ""},
		{"auth 401", fmt.Errorf("401 Unauthorized"), model.ErrorAuth},
		{"auth 403", fmt.Errorf("403 Forbidden"), model.ErrorAuth},
		{"auth authentication", fmt.Errorf("authentication failed"), model.ErrorAuth},
		{"auth unauthorized", fmt.Errorf("unauthorized access"), model.ErrorAuth},
		{"auth permission denied", fmt.Errorf("permission denied"), model.ErrorAuth},
		{"auth token", fmt.Errorf("invalid token"), model.ErrorAuth},
		{"auth credential", fmt.Errorf("credential error"), model.ErrorAuth},
		{"network timeout", fmt.Errorf("connection timeout"), model.ErrorNetwork},
		{"network connection refused", fmt.Errorf("connection refused"), model.ErrorNetwork},
		{"network dial tcp", fmt.Errorf("dial tcp: lookup example.com"), model.ErrorNetwork},
		{"network network", fmt.Errorf("network error"), model.ErrorNetwork},
		{"network i/o error", fmt.Errorf("i/o error"), model.ErrorNetwork},
		{"network eof", fmt.Errorf("unexpected eof"), model.ErrorNetwork},
		{"network no route", fmt.Errorf("no route to host"), model.ErrorNetwork},
		{"config not found", fmt.Errorf("repository not found"), model.ErrorConfig},
		{"config does not exist", fmt.Errorf("branch does not exist"), model.ErrorConfig},
		{"config repository not found", fmt.Errorf("repository not found"), model.ErrorConfig},
		{"config branch not found", fmt.Errorf("branch not found"), model.ErrorConfig},
		{"git non-fast-forward", fmt.Errorf("non-fast-forward"), model.ErrorGit},
		{"git conflict", fmt.Errorf("merge conflict"), model.ErrorGit},
		{"git rejected", fmt.Errorf("push rejected"), model.ErrorGit},
		{"git failed to push", fmt.Errorf("failed to push"), model.ErrorGit},
		{"git fetch first", fmt.Errorf("fetch first"), model.ErrorGit},
		{"unknown error", fmt.Errorf("some random error"), model.ErrorUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			assert.Equal(t, tt.expected, result, "ClassifyError(%v)", tt.err)
		})
	}
}

func TestExecute_WithContext(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run, err := exec.Execute(ctx, task, "manual", nil)
	require.Error(t, err, "expected error for cancelled context")
	require.NotNil(t, run, "expected non-nil run even on error")
}

func TestExecute_WithWebhookEventID(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	webhookEventID := uint(123)
	ctx := context.Background()
	_, _ = exec.Execute(ctx, task, "webhook", &webhookEventID)

	// Verify run was created with webhook event ID
	require.NotEmpty(t, svc.runs, "expected run to be created")

	run := svc.runs[0]
	require.NotNil(t, run.WebhookEventID, "expected webhook event ID to be set")

	assert.Equal(t, uint(123), *run.WebhookEventID, "expected webhook event ID 123")
}

func TestBeginStep(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	// Create a run
	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
	}

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "CreateRun failed")

	// Begin a step
	step := exec.beginStep(run.ID, "fetch")
	require.NotNil(t, step, "expected non-nil step")

	assert.Equal(t, run.ID, step.RunID, "expected run ID %d", run.ID)
	assert.Equal(t, "fetch", step.StepName, "expected step name 'fetch'")
	assert.Equal(t, "running", step.Status, "expected status 'running'")
}

func TestFailStep(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	// Create a run
	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
	}

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "CreateRun failed")

	// Begin a step
	step := exec.beginStep(run.ID, "fetch")
	require.NotNil(t, step, "expected non-nil step")

	// Fail the step
	exec.failStep(step, fmt.Errorf("fetch failed"))

	assert.Equal(t, "failed", step.Status, "expected status 'failed'")
	assert.Equal(t, "fetch failed", step.ErrorMsg, "expected error message 'fetch failed'")
}

func TestCompleteStep(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	// Create a run
	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
	}

	run, err := svc.CreateRun(task, "manual", nil)
	require.NoError(t, err, "CreateRun failed")

	// Begin a step
	step := exec.beginStep(run.ID, "fetch")
	require.NotNil(t, step, "expected non-nil step")

	// Complete the step
	exec.completeStep(step, "details")

	assert.Equal(t, "success", step.Status, "expected status 'success'")
}

func TestAuthConfig_WithPlatform(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{
		AccessToken: "",
		PlatformID:  1,
	}

	config := exec.authConfig(context.Background(), repo, nil)
	// mockService returns a platform with AccessToken="test-token"
	assert.Equal(t, "test-token", config.Password, "expected platform token as password")
}

func TestAuthConfig_RepoTokenOverridesPlatform(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{
		AccessToken: "repo-specific-token",
		PlatformID:  1,
	}

	config := exec.authConfig(context.Background(), repo, nil)
	// Repo token should take precedence
	assert.Equal(t, "repo-specific-token", config.Password, "expected repo token as password")
}

func TestAuthConfig_NoPlatformID(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{
		AccessToken: "",
		PlatformID:  0,
	}

	config := exec.authConfig(context.Background(), repo, nil)
	assert.Equal(t, "none", string(config.Type), "expected auth type 'none'")
}

func TestCloneRepo_WithForceFlag(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		SourceBranch: "main",
		GitForce:     true,
	}

	repo := &model.Repo{
		CloneURL: "https://github.com/test/repo.git",
	}

	// This will fail because we don't have a real git backend,
	// but it exercises the depth=0 path
	_ = exec.cloneRepo(context.Background(), "/tmp/test-clone", repo, task, nil)
}

func TestCloneRepo_ShallowClone(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		SourceBranch: "main",
		GitForce:     false,
	}

	repo := &model.Repo{
		CloneURL: "https://github.com/test/repo.git",
	}

	// This will fail because we don't have a real git backend,
	// but it exercises the depth=1 path
	_ = exec.cloneRepo(context.Background(), "/tmp/test-clone", repo, task, nil)
}

func TestNewExecutor_WithBackendConfig(t *testing.T) {
	svc := &mockServiceWithConfig{
		mockService: newMockService(),
		backendType: "gogit",
	}
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")
	require.NotNil(t, exec, "expected non-nil executor")
}

func TestNewExecutor_InvalidBackend(t *testing.T) {
	svc := &mockServiceWithConfig{
		mockService: newMockService(),
		backendType: "invalid-backend-type",
	}
	_, err := NewExecutor(svc)
	require.Error(t, err, "expected error for invalid backend type")
}

// mockServiceWithConfig wraps mockService to return a specific config
type mockServiceWithConfig struct {
	*mockService
	backendType string
}

func (m *mockServiceWithConfig) GetConfig() *model.Config {
	return &model.Config{
		Git: model.GitConfig{
			Backend: m.backendType,
		},
		Sync: model.SyncConfig{
			DefaultTimeout: 300,
			RetryCount:     3,
		},
	}
}

func TestExecute_CompleteRunError(t *testing.T) {
	svc := newMockService()
	svc.completeErr = fmt.Errorf("db write failed")
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "missing-repo", // Will fail, triggering complete
		TargetRepoKey: "target-repo",
	}

	ctx := context.Background()
	run, err := exec.Execute(ctx, task, "manual", nil)
	// Should still return the run even with complete error
	require.NotNil(t, run, "expected non-nil run")
	require.Error(t, err, "expected error for missing source repo")
}

func TestExecute_UpdateTaskLastRunError(t *testing.T) {
	svc := newMockService()
	svc.updateErr = fmt.Errorf("db update failed")
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "missing-repo", // Will fail, triggering update
		TargetRepoKey: "target-repo",
	}

	ctx := context.Background()
	run, _ := exec.Execute(ctx, task, "manual", nil)
	require.NotNil(t, run, "expected non-nil run")
}

func TestExecute_SourceRepoQueryError(t *testing.T) {
	svc := newMockService()
	svc.repoErr = fmt.Errorf("database connection lost")
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "source-repo",
		TargetRepoKey: "target-repo",
		SourceBranch:  "main",
		TargetBranch:  "main",
	}

	ctx := context.Background()
	run, err := exec.Execute(ctx, task, "manual", nil)
	require.Error(t, err, "expected error when repo query fails")
	require.NotNil(t, run, "expected non-nil run")
	assert.Equal(t, model.StatusFailed, run.Status, "expected status 'failed'")
	assert.Equal(t, model.ErrorUnknown, run.ErrorType, "expected error type '%s'", model.ErrorUnknown)
}

func TestFailRun(t *testing.T) {
	run := &model.SyncRun{
		ID:     1,
		Status: model.StatusRunning,
	}

	err := fmt.Errorf("401 authentication failed")
	resultRun, resultErr := failRun(run, err)

	assert.Equal(t, model.StatusFailed, resultRun.Status, "expected status 'failed'")
	assert.Equal(t, "401 authentication failed", resultRun.ErrorMessage, "expected error message")
	assert.Equal(t, model.ErrorAuth, resultRun.ErrorType, "expected error type")
	require.NotNil(t, resultErr, "expected non-nil error")
}

func TestFetchRepo(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		SourceBranch: "main",
		GitTags:      true,
		GitPrune:     true,
	}

	repo := &model.Repo{
		CloneURL: "https://github.com/test/repo.git",
	}

	// This will fail because we don't have a real git repo,
	// but it exercises the fetchRepo code path
	_ = exec.fetchRepo(context.Background(), "/tmp/nonexistent", task, repo, nil)
}

func TestEnsureRemote(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	repo := &model.Repo{
		CloneURL: "https://github.com/test/target.git",
	}

	// This will fail because we don't have a real git repo,
	// but it exercises the ensureRemote code path
	_ = exec.ensureRemote(context.Background(), "/tmp/nonexistent", repo)
}

func TestPush(t *testing.T) {
	svc := newMockService()
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		SourceBranch: "main",
		TargetBranch: "main",
		GitForce:     false,
	}

	repo := &model.Repo{
		CloneURL: "https://github.com/test/target.git",
	}

	// This will fail because we don't have a real git repo,
	// but it exercises the push code path
	_ = exec.push(context.Background(), "/tmp/nonexistent", task, repo, nil)
}

func TestExecute_WithDefaultTimeout(t *testing.T) {
	svc := &mockServiceWithConfig{
		mockService: newMockService(),
		backendType: "",
	}
	svc.config = &model.Config{
		Sync: model.SyncConfig{
			DefaultTimeout: 0, // Should default to 300
			RetryCount:     1,
		},
	}
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "missing-repo",
		TargetRepoKey: "target-repo",
	}

	ctx := context.Background()
	run, _ := exec.Execute(ctx, task, "manual", nil)
	require.NotNil(t, run, "expected non-nil run")
}

func TestExecute_WithZeroRetryCount(t *testing.T) {
	svc := &mockServiceWithConfig{
		mockService: newMockService(),
		backendType: "",
	}
	svc.config = &model.Config{
		Sync: model.SyncConfig{
			DefaultTimeout: 300,
			RetryCount:     0, // Should default to 3
		},
	}
	exec, err := NewExecutor(svc)
	require.NoError(t, err, "NewExecutor failed")

	task := &model.SyncTask{
		Key:           "test-task",
		Name:          "Test Task",
		SourceRepoKey: "missing-repo",
		TargetRepoKey: "target-repo",
	}

	ctx := context.Background()
	run, _ := exec.Execute(ctx, task, "manual", nil)
	require.NotNil(t, run, "expected non-nil run")
}

func TestRemoteURLsEqual(t *testing.T) {
	assert.True(t, remoteURLsEqual("https://a/b.git", "https://a/b.git"))
	assert.True(t, remoteURLsEqual("https://a/b.git/", "https://a/b.git"))
	assert.False(t, remoteURLsEqual("https://a/b.git", "https://a/c.git"))
	assert.False(t, remoteURLsEqual("https://old/x.git", "https://new/x.git"))
}

func TestDefaultBranchOf(t *testing.T) {
	assert.Equal(t, "develop", defaultBranchOf(&model.Repo{DefaultBranch: "develop"}))
	assert.Equal(t, "main", defaultBranchOf(&model.Repo{DefaultBranch: ""}))
	assert.Equal(t, "main", defaultBranchOf(nil))
}
