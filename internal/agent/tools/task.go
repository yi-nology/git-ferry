package tools

import (
	"context"
	"fmt"
)

// ===== 任务 =====

type listTasksInput struct {
	RepoKey string `json:"repo_key,omitempty" jsonschema:"按仓库 key 过滤,可选"`
	Page    int    `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
}

type taskSummary struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	SourceRepo   string `json:"source_repo"`
	SourceBranch string `json:"source_branch"`
	TargetRepo   string `json:"target_repo"`
	TargetBranch string `json:"target_branch"`
	Cron         string `json:"cron,omitempty"`
	Enabled      bool   `json:"enabled"`
	LastStatus   string `json:"last_status,omitempty"`
}

func (r *Registry) listTasks(ctx context.Context, in listTasksInput) (string, error) {
	list, total, err := r.svc.ListTasks(ctx, in.RepoKey, pageBounds(in.Page), pageLimit)
	if err != nil {
		return fmt.Sprintf(`{"error":"查询任务列表失败: %s"}`, err), nil
	}
	tasks := []taskSummary{}
	for _, tk := range list {
		tasks = append(tasks, taskSummary{
			Key: tk.Key, Name: tk.Name,
			SourceRepo: tk.SourceRepoKey, SourceBranch: tk.SourceBranch,
			TargetRepo: tk.TargetRepoKey, TargetBranch: tk.TargetBranch,
			Cron: tk.Cron, Enabled: tk.Enabled, LastStatus: tk.LastStatus,
		})
	}
	return marshalJSON(map[string]any{"total": total, "tasks": tasks}), nil
}

func (r *Registry) getTask(ctx context.Context, in keyInput) (string, error) {
	tk, err := r.svc.GetTask(ctx, in.Key)
	if err != nil || tk == nil {
		return fmt.Sprintf(`{"found":false,"message":"未找到任务 %q"}`, in.Key), nil
	}
	return marshalJSON(taskSummary{
		Key: tk.Key, Name: tk.Name,
		SourceRepo: tk.SourceRepoKey, SourceBranch: tk.SourceBranch,
		TargetRepo: tk.TargetRepoKey, TargetBranch: tk.TargetBranch,
		Cron: tk.Cron, Enabled: tk.Enabled, LastStatus: tk.LastStatus,
	}), nil
}
