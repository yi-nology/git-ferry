package git_sync

import (
	"fmt"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// newIssueProvider 由平台+仓库 token 构造 SDK provider(issues/metadata 等共用)。
func newIssueProvider(plat *corebridge.Platform, repoToken string) (sdkprov.Provider, error) {
	token := repoToken
	if token == "" {
		token = plat.AccessToken
	}
	cfg := sdkprov.Config{
		Platform: sdkprov.Platform(plat.Type),
		BaseURL:  plat.APIURL,
		Token:    token,
		SkipTLS:  plat.SkipTLSVerify,
	}
	p, err := sdkprov.NewProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}
	return p, nil
}
