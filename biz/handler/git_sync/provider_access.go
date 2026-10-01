package git_sync

import (
	"fmt"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// newIssueProvider 由平台+仓库 token 构造 SDK provider(issues/metadata 等共用)。
// 经 core ProviderForPlatform:Manager 缓存 + GitHub App installation token 解析;
// repoToken 非空优先,空则按平台解析(原先手写 repo token→平台 token 回退已收敛)。
func newIssueProvider(plat *corebridge.Platform, repoToken string) (sdkprov.Provider, error) {
	svc := GetSyncService()
	if svc == nil {
		return nil, fmt.Errorf("create provider: sync service unavailable")
	}
	p, err := svc.ProviderForPlatform(plat, repoToken)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}
	return p, nil
}
