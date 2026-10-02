package git_sync

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// OpsTodo GET /api/v1/ops/todo
// 聚合健康 attention + 孤儿仓库 + RPO 超标，按优先级输出可执行队列。
// 聚合规则在 core Service.OpsTodo；壳只做计数与响应包装。
func OpsTodo(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	items := svc.OpsTodo(ctx)
	byKind := map[string]int{}
	for i := range items {
		byKind[items[i].Kind]++
	}
	response.Success(c, map[string]any{
		"items":        items,
		"total":        len(items),
		"by_kind":      byKind,
		"generated_at": time.Now().Format(time.RFC3339),
	})
}
