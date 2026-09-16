package tools

import (
	"context"
	"fmt"
)

// ===== 平台 / Webhook / 系统概览 =====

type platformSummary struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	InstanceURL string `json:"instance_url,omitempty"`
	Status      string `json:"status"`
	IsDefault   bool   `json:"is_default"`
	RepoCount   int    `json:"repo_count"`
	LastTest    string `json:"last_test_result,omitempty"`
}

func (r *Registry) listPlatforms(ctx context.Context, _ emptyInput) (string, error) {
	list, err := r.svc.ListPlatforms(ctx)
	if err != nil {
		return fmt.Sprintf(`{"error":"查询平台失败: %s"}`, err), nil
	}
	platforms := []platformSummary{}
	for _, p := range list {
		platforms = append(platforms, platformSummary{
			Key: p.Key, Name: p.Name, Type: p.Type,
			InstanceURL: p.InstanceURL, Status: p.Status,
			IsDefault: p.IsDefault, RepoCount: p.RepoCount,
			LastTest: p.LastTestResult,
		})
	}
	return marshalJSON(map[string]any{"platforms": platforms}), nil
}

type listRulesInput struct {
	RepoKey string `json:"repo_key,omitempty" jsonschema:"按仓库 key 过滤,可选"`
}

func (r *Registry) listRules(ctx context.Context, in listRulesInput) (string, error) {
	list, err := r.svc.ListRules(ctx, in.RepoKey)
	if err != nil {
		return fmt.Sprintf(`{"error":"查询 Webhook 规则失败: %s"}`, err), nil
	}
	rules := []map[string]any{}
	for _, w := range list {
		rules = append(rules, map[string]any{
			"id": w.ID, "name": w.Name, "repo_key": w.RepoKey,
			"event_type": w.EventType, "action": w.Action,
			"enabled": w.Enabled,
		})
	}
	return marshalJSON(map[string]any{"rules": rules}), nil
}

// emptyInput 无参数工具的占位输入。
type emptyInput struct{}

func (r *Registry) getSystemOverview(ctx context.Context, _ emptyInput) (string, error) {
	repoCount, err := r.svc.CountRepos()
	if err != nil {
		return fmt.Sprintf(`{"error":"统计仓库失败: %s"}`, err), nil
	}
	taskStatus, err := r.svc.CountTasksByStatus()
	if err != nil {
		return fmt.Sprintf(`{"error":"统计任务失败: %s"}`, err), nil
	}
	return marshalJSON(map[string]any{
		"repo_count":      repoCount,
		"tasks_by_status": taskStatus,
		"health":          r.svc.HealthCheck(),
	}), nil
}
