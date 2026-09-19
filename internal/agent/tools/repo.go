package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// Registry 工具注册表:构建 eino 工具集,并支持按名调用(确认执行路径复用)。
type Registry struct {
	svc    SyncService
	byName map[string]tool.InvokableTool
}

func NewRegistry(svc SyncService) *Registry {
	r := &Registry{svc: svc, byName: map[string]tool.InvokableTool{}}
	must := func(t tool.InvokableTool, err error) tool.InvokableTool {
		if err != nil {
			panic(err) // 仅构造期:输入 struct 定义错误属编程错误
		}
		return t
	}
	r.add(must(utils.InferTool("list_repos", "查询同步仓库列表,支持关键字过滤", r.listRepos)))
	r.add(must(utils.InferTool("get_repo", "查询单个仓库详情(含状态、平台、分支)", r.getRepo)))
	r.add(must(utils.InferTool("list_branches", "列出仓库在源平台的分支", r.listBranches)))
	r.add(must(utils.InferTool("list_tasks", "查询同步任务列表", r.listTasks)))
	r.add(must(utils.InferTool("get_task", "查询单个同步任务详情", r.getTask)))
	r.add(must(utils.InferTool("list_sync_history", "查询任务最近同步执行历史", r.listHistory)))
	r.add(must(utils.InferTool("get_run_detail", "查询单次同步执行的详情(含执行步骤与错误链)", r.getRunDetail)))
	r.add(must(utils.InferTool("list_platforms", "查询平台列表及状态", r.listPlatforms)))
	r.add(must(utils.InferTool("list_webhook_rules", "查询 Webhook 同步规则", r.listRules)))
	r.add(must(utils.InferTool("get_system_overview", "系统概览(仓库数/任务状态/健康检查)", r.getSystemOverview)))
	// 危险工具(需用户确认)
	r.add(must(utils.InferTool("run_task", "立即执行一次同步任务(危险操作,需用户确认)", r.runTask)))
	r.add(must(utils.InferTool("test_repo_connection", "测试仓库连通性(危险操作,需用户确认)", r.testRepoConnection)))
	r.add(must(utils.InferTool("test_platform_connection", "测试平台连通性(危险操作,需用户确认)", r.testPlatformConnection)))
	return r
}

func (r *Registry) add(t tool.InvokableTool) {
	info, err := t.Info(context.Background())
	if err != nil {
		panic(err)
	}
	r.byName[info.Name] = t
}

func (r *Registry) ByName(name string) tool.InvokableTool { return r.byName[name] }

// Names 返回全部工具名。
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	return out
}

// ===== 仓库 =====

type listReposInput struct {
	Keyword string `json:"keyword,omitempty" jsonschema:"按名称或克隆地址过滤的关键字,可选"`
	Page    int    `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
}

type repoSummary struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	Owner         string `json:"owner,omitempty"`
	Status        string `json:"status"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

func (r *Registry) listRepos(ctx context.Context, in listReposInput) (map[string]any, error) {
	// 始终传非 nil filter:core 的 ListWithFilter 不做空指针防护
	filter := &corebridge.RepoFilter{}
	if in.Keyword != "" {
		filter.Search = in.Keyword
	}
	list, total, err := r.svc.ListReposWithFilter(ctx, pageBounds(in.Page), pageLimit, filter)
	if err != nil {
		return nil, fmt.Errorf("查询仓库列表失败: %w", err)
	}
	repos := []repoSummary{}
	for _, rp := range list {
		repos = append(repos, repoSummary{
			Key: rp.Key, Name: rp.Name, Platform: rp.Platform,
			Owner: rp.PlatformOwner, Status: rp.Status,
			CloneURL: rp.CloneURL, DefaultBranch: rp.DefaultBranch,
		})
	}
	return map[string]any{"total": total, "page": maxInt(in.Page, 1), "repos": repos}, nil
}

type keyInput struct {
	Key string `json:"key" jsonschema:"仓库 key"`
}

func (r *Registry) getRepo(ctx context.Context, in keyInput) (string, error) {
	rp, err := r.svc.GetRepo(ctx, in.Key)
	if err != nil || rp == nil {
		return fmt.Sprintf(`{"found":false,"message":"未找到仓库 %q"}`, in.Key), nil
	}
	return marshalJSON(repoSummary{
		Key: rp.Key, Name: rp.Name, Platform: rp.Platform,
		Owner: rp.PlatformOwner, Status: rp.Status,
		CloneURL: rp.CloneURL, DefaultBranch: rp.DefaultBranch,
	}), nil
}

type branchInput struct {
	Key string `json:"key" jsonschema:"仓库 key"`
}

func (r *Registry) listBranches(ctx context.Context, in branchInput) (string, error) {
	branches, err := r.svc.ListBranches(ctx, in.Key)
	if err != nil {
		return fmt.Sprintf(`{"error":"查询分支失败: %s"}`, err), nil
	}
	if len(branches) > pageLimit {
		branches = branches[:pageLimit]
	}
	return marshalJSON(map[string]any{"branches": branches}), nil
}
