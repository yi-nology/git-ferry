// Package toolstest 提供跨包共用的测试基建:SyncService mock 与脚本化假模型。
package toolstest

import (
	"context"
	"sync"

	coremodel "github.com/yi-nology/git-ferry-core/model"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/go-git-platform/provider"
)

// Mock 可配置的 SyncService 假实现。
type Mock struct {
	Repos     []*corebridge.Repo
	RepoTotal int64
	RepoErr   error

	Tasks     []*corebridge.SyncTask
	TaskTotal int64
	// TaskErr 注入任务查询失败(健康维度会因此整段查不出来)。
	TaskErr error

	Runs     []*corebridge.SyncRun
	RunTotal int64

	Platforms []*corebridge.Platform
	Rules     []*corebridge.WebhookRule

	TaskStatus map[string]int64
	Health     map[string]string

	ConnResult  *coremodel.TestConnectionResult
	PlatConnRes *provider.TestConnectionResult

	RunTaskErr error

	// BackupDirPath 冷备目录（get_metadata_snapshots 用）
	BackupDirPath string

	// P0-P5 可注入结果
	RPO       *corebridge.RPOReport
	RPOErr    error
	Integrity *corebridge.ManifestVerifyResult
	Drift     *corebridge.DriftReport

	// HistoryRuns 历史同步执行记录(分页用)。
	HistoryRuns      []*corebridge.HistoryRunDTO
	HistoryRunsTotal int64
	// HistoryRunsOffset/HistoryRunsLimit 记录上次调用的分页参数,
	// 便于断言工具确实按页取数(而不是硬编码 50 条)。
	HistoryRunsOffset int
	HistoryRunsLimit  int

	// mu 保护在请求 goroutine 与测试 goroutine 间并发的字段。
	mu             sync.Mutex
	lastRunTaskKey string

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
	if m.TaskErr != nil {
		return nil, 0, m.TaskErr
	}
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
	m.mu.Lock()
	m.lastRunTaskKey = taskKey
	m.mu.Unlock()
	return m.RunTaskErr
}

// RunTaskKey 返回最近一次触发同步的任务 key(-race 下并发安全)。
func (m *Mock) RunTaskKey() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRunTaskKey
}

// ===== P0-P5:灾备 / RPO / 漂移 / 审计链 =====

func (m *Mock) RPOReport(int64) (*corebridge.RPOReport, error) {
	if m.RPOErr != nil {
		return nil, m.RPOErr
	}
	if m.RPO == nil {
		return &corebridge.RPOReport{Metrics: []corebridge.RPOMetric{}}, nil
	}
	return m.RPO, nil
}

func (m *Mock) VerifyBackupManifest() (*corebridge.ManifestVerifyResult, error) {
	if m.Integrity != nil {
		return m.Integrity, nil
	}
	return &corebridge.ManifestVerifyResult{OK: true, Message: "ok"}, nil
}

func (m *Mock) DetectDrift(context.Context, []string) (*corebridge.DriftReport, error) {
	if m.Drift != nil {
		return m.Drift, nil
	}
	return &corebridge.DriftReport{Items: []corebridge.DriftItem{}}, nil
}

func (m *Mock) VerifyAuditChain() (*corebridge.AuditChainResult, error) {
	return &corebridge.AuditChainResult{OK: true, Checked: 0, Message: "ok"}, nil
}

func (m *Mock) RunDRDrill(context.Context, string) (*corebridge.DrillReport, error) {
	return &corebridge.DrillReport{Success: true, EstRTO: "1s"}, nil
}

func (m *Mock) BatchDRDrill(context.Context, []string, int) ([]*corebridge.DrillReport, map[string]any, error) {
	return []*corebridge.DrillReport{{Success: true}},
		map[string]any{"total": 1, "success": 1, "failed": 0}, nil
}

func (m *Mock) BackupDir() string { return m.BackupDirPath }

// ===== 历史同步(histsync) =====

// ListHistoryChannels 返回假的历史同步通道列表。
func (m *Mock) ListHistoryChannels(_ context.Context, _, _ int) ([]*corebridge.HistoryChannelDTO, int64, error) {
	return []*corebridge.HistoryChannelDTO{}, 0, nil
}

// HistoryPlan 返回假的按通道计划。
func (m *Mock) HistoryPlan(_ context.Context, _ uint) (*corebridge.HistoryPlanDTO, error) {
	return &corebridge.HistoryPlanDTO{Summary: corebridge.HistorySummary{}}, nil
}

// HistoryResolve 返回假的按通道解法结果。
func (m *Mock) HistoryResolve(_ context.Context, _ uint) (*corebridge.HistoryPlanDTO, error) {
	return &corebridge.HistoryPlanDTO{Summary: corebridge.HistorySummary{}}, nil
}

// ListHistoryRuns 返回假的执行记录列表。
func (m *Mock) ListHistoryRuns(_ context.Context, _ uint, offset, limit int) ([]*corebridge.HistoryRunDTO, int64, error) {
	m.HistoryRunsOffset = offset
	m.HistoryRunsLimit = limit
	if m.HistoryRuns == nil {
		return []*corebridge.HistoryRunDTO{}, 0, nil
	}
	// 模拟真分页:按 offset/limit 切片,total 始终是全量条数。
	if offset > len(m.HistoryRuns) {
		offset = len(m.HistoryRuns)
	}
	end := len(m.HistoryRuns)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return m.HistoryRuns[offset:end], m.HistoryRunsTotal, nil
}

// VerifyHistoryChannelChain 返回假的链校验结果。
func (m *Mock) VerifyHistoryChannelChain(_ context.Context, _ uint) (*corebridge.HistoryChainResult, error) {
	return &corebridge.HistoryChainResult{OK: true, Message: "audit chain intact"}, nil
}

// ListHistoryOverrides 返回假的裁决列表。
func (m *Mock) ListHistoryOverrides(_ context.Context, _ uint) ([]*corebridge.HistoryOverrideDTO, error) {
	return []*corebridge.HistoryOverrideDTO{}, nil
}

// SetHistoryOverride 返回假的裁决写入结果。
func (m *Mock) SetHistoryOverride(_ context.Context, _ uint, in corebridge.HistoryOverrideInput) (*corebridge.HistoryOverrideDTO, error) { //nolint:gocritic // hugeParam: 签名由 SyncService 接口锁定,而该接口镜像 core 的 SetHistoryOverride(值类型)
	return &corebridge.HistoryOverrideDTO{Path: in.Path, Verdict: in.Verdict}, nil
}

// HistoryPreview 返回假的预检报告。
func (m *Mock) HistoryPreview(_ context.Context, _ uint) (*corebridge.PreviewReport, error) {
	return &corebridge.PreviewReport{ChannelID: 0}, nil
}

// HistorySync 返回假的一次同步结果。
//
// **Push / Preview 必须非 nil**:调用方会直接取 rep.Push.Heads / rep.Preview.Summary
// (空报告会空指针崩掉)。这正是之前 mock 返回空结构体时踩的坑。
func (m *Mock) HistorySync(_ context.Context, _ corebridge.HistorySyncOptions) (*corebridge.HistorySyncReport, error) {
	return &corebridge.HistorySyncReport{
		Preview: &corebridge.PreviewReport{},
		Apply:   &corebridge.HistoryRunDTO{},
		Push:    &corebridge.HistoryPushDTO{Heads: map[string]string{}},
	}, nil
}

// RotateHistoryTargetCredential 返回假的凭据轮换结果。
func (m *Mock) RotateHistoryTargetCredential(_ context.Context, _ uint, in corebridge.HistoryCredentialInput) (*corebridge.HistoryTargetDTO, error) {
	return &corebridge.HistoryTargetDTO{CredType: in.CredType, HasCredential: in.Credential != ""}, nil
}
