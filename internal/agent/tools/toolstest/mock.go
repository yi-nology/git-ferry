// Package toolstest 提供跨包共用的测试基建:SyncService mock 与脚本化假模型。
package toolstest

import (
	"context"

	"github.com/yi-nology/git-platform-sdk/provider"
	coremodel "github.com/yi-nology/git-sync-core/model"
	"github.com/yi-nology/git-sync-service/internal/corebridge"
)

// Mock 可配置的 SyncService 假实现。
type Mock struct {
	Repos     []*corebridge.Repo
	RepoTotal int64
	RepoErr   error

	Tasks     []*corebridge.SyncTask
	TaskTotal int64

	Runs     []*corebridge.SyncRun
	RunTotal int64

	Platforms []*corebridge.Platform
	Rules     []*corebridge.WebhookRule

	TaskStatus map[string]int64
	Health     map[string]string

	ConnResult  *coremodel.TestConnectionResult
	PlatConnRes *provider.TestConnectionResult

	LastRunTaskKey string
	RunTaskErr     error

	LastFilter *corebridge.RepoFilter
}

func NewMock() *Mock {
	return &Mock{
		TaskStatus: map[string]int64{},
		Health:     map[string]string{"database": "ok"},
	}
}

func (m *Mock) ListReposWithFilter(_ context.Context, _, _ int, f *corebridge.RepoFilter) ([]*corebridge.Repo, int64, error) {
	m.LastFilter = f
	return m.Repos, m.RepoTotal, m.RepoErr
}

func (m *Mock) CountRepos() (int64, error) { return m.RepoTotal, m.RepoErr }

func (m *Mock) GetRepo(_ context.Context, key string) (*corebridge.Repo, error) {
	for _, r := range m.Repos {
		if r.Key == key {
			return r, nil
		}
	}
	return nil, corebridge.ErrRepoNotFound
}

func (m *Mock) ListBranches(context.Context, string) ([]string, error) {
	return []string{"main", "dev"}, nil
}

func (m *Mock) TestConnection(context.Context, string) (*coremodel.TestConnectionResult, error) {
	if m.ConnResult == nil {
		return &coremodel.TestConnectionResult{Success: true, Message: "ok"}, nil
	}
	return m.ConnResult, nil
}

func (m *Mock) ListTasks(context.Context, string, int, int) ([]*corebridge.SyncTask, int64, error) {
	return m.Tasks, m.TaskTotal, nil
}

func (m *Mock) GetTask(_ context.Context, key string) (*corebridge.SyncTask, error) {
	for _, tk := range m.Tasks {
		if tk.Key == key {
			return tk, nil
		}
	}
	return nil, corebridge.ErrTaskNotFound
}

func (m *Mock) ListHistory(context.Context, string, int, int) ([]*corebridge.SyncRun, int64, error) {
	return m.Runs, m.RunTotal, nil
}

func (m *Mock) ListPlatforms(context.Context) ([]*corebridge.Platform, error) {
	return m.Platforms, nil
}

func (m *Mock) TestPlatformConnection(context.Context, string) (*provider.TestConnectionResult, error) {
	if m.PlatConnRes == nil {
		return &provider.TestConnectionResult{Connected: true, Message: "ok"}, nil
	}
	return m.PlatConnRes, nil
}

func (m *Mock) ListRules(context.Context, string) ([]*corebridge.WebhookRule, error) {
	return m.Rules, nil
}

func (m *Mock) CountTasksByStatus() (map[string]int64, error) { return m.TaskStatus, nil }

func (m *Mock) HealthCheck() map[string]string { return m.Health }

func (m *Mock) RunTaskWithTrigger(_ context.Context, taskKey, _ string, _ *uint) error {
	m.LastRunTaskKey = taskKey
	return m.RunTaskErr
}
