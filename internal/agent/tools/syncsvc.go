// Package tools 把 corebridge 能力包装为 eino 工具。
// 所有列表类工具每页硬上限 20 条,控制 token 用量;凭据字段永不进入工具输出。
// 依赖方向:tools → corebridge;会话能力经 SessionScope 接口注入,tools 不感知 agent 包。
package tools

import (
	"context"
	"encoding/json"

	"github.com/yi-nology/git-platform-sdk/provider"
	coremodel "github.com/yi-nology/git-ferry-core/model"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// pageLimit 工具返回列表的单页上限。
const pageLimit = 20

// SyncService 工具所需的 Service 能力子集(*corebridge.Service 天然满足)。
// 定义窄接口便于单测 mock,也避免 tools 依赖 corebridge 具体构造。
type SyncService interface {
	ListReposWithFilter(ctx context.Context, offset, limit int, filter *corebridge.RepoFilter) ([]*corebridge.Repo, int64, error)
	CountRepos() (int64, error)
	GetRepo(ctx context.Context, key string) (*corebridge.Repo, error)
	ListBranches(ctx context.Context, repoKey string) ([]string, error)
	TestConnection(ctx context.Context, repoKey string) (*coremodel.TestConnectionResult, error)

	ListTasks(ctx context.Context, repoKey string, offset, limit int) ([]*corebridge.SyncTask, int64, error)
	GetTask(ctx context.Context, key string) (*corebridge.SyncTask, error)
	ListHistory(ctx context.Context, taskKey string, offset, limit int) ([]*corebridge.SyncRun, int64, error)

	ListPlatforms(ctx context.Context) ([]*corebridge.Platform, error)
	TestPlatformConnection(ctx context.Context, key string) (*provider.TestConnectionResult, error)

	ListRules(ctx context.Context, repoKey string) ([]*corebridge.WebhookRule, error)

	CountTasksByStatus() (map[string]int64, error)
	HealthCheck() map[string]string
	RunTaskWithTrigger(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error
}

// 编译期断言:corebridge.Service 必须满足 SyncService,签名漂移立即暴露。
var _ SyncService = (*corebridge.Service)(nil)

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pageBounds(page int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * pageLimit
}

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
