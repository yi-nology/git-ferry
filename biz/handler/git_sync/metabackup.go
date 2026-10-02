package git_sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// 备份编排:平台取数(provider 可选能力)+ 本地落盘,metadata 快照与 gists 备份
// 端点共用(同包两个调用方)。能力路由按平台规约走 Capabilities() 而非裸类型断言。

// backupGists 拉取 token 用户可见的 gists(分页到 maxGists 为止)并逐个写 JSON 文件。
// 返回成功落盘条数与逐条 warning;平台不声明 Gists 能力时报错。
func backupGists(ctx context.Context, prov sdkprov.Provider, destDir string, maxGists int) (count int, warnings []string, err error) {
	if !prov.Capabilities().Gists {
		return 0, nil, errors.New("platform does not support gists")
	}
	gm := prov.(sdkprov.GistManager)
	if maxGists <= 0 {
		maxGists = 200
	}
	perPage := sdkprov.MaxPerPage
	maxPages := (maxGists + perPage - 1) / perPage
	// 空页终止 + 到 max 即 ErrStopIteration;预算 +1 页用于观测空页。
	var gists []*sdkprov.Gist
	err = sdkprov.EachBounded(ctx,
		func(ctx context.Context, page int) ([]*sdkprov.Gist, error) {
			return gm.ListMyGists(ctx, page, perPage)
		}, maxPages+1,
		func(g *sdkprov.Gist) error {
			if len(gists) >= maxGists {
				return sdkprov.ErrStopIteration
			}
			gists = append(gists, g)
			return nil
		})
	if err != nil {
		return 0, nil, err
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return 0, nil, err
	}
	for _, g := range gists {
		if g == nil {
			continue
		}
		data, merr := json.MarshalIndent(g, "", "  ")
		if merr != nil {
			warnings = append(warnings, g.ID+": marshal")
			continue
		}
		name := textutil.SanitizePathToken(g.ID) + ".json"
		if werr := os.WriteFile(filepath.Join(destDir, name), data, 0o600); werr != nil {
			warnings = append(warnings, g.ID+": write")
			continue
		}
		count++
	}
	return count, warnings, nil
}

// downloadReleaseAssets 下载 release 附件到 destDir(流式写盘)。
// releases 非空时直接复用(带 Assets 元数据,不重复请求);否则现拉 ListReleases。
// 文件名 tag__name(均经路径清洗),上限 maxAssets,warning 文案逐附件。
// 平台不声明 ReleaseAssets 能力时报错。
func downloadReleaseAssets(ctx context.Context, prov sdkprov.Provider, owner, repo string,
	releases []*sdkprov.ReleaseInfo, destDir string, maxAssets int) (saved, warnings []string, err error) {
	if !prov.Capabilities().ReleaseAssets {
		return nil, nil, errors.New("platform does not support release assets")
	}
	ram := prov.(sdkprov.ReleaseAssetManager)
	if maxAssets <= 0 {
		maxAssets = 50
	}
	if len(releases) == 0 {
		releases, err = prov.ListReleases(ctx, owner, repo)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return nil, nil, err
	}
	count := 0
	for _, rel := range releases {
		if rel == nil {
			continue
		}
		for _, asset := range rel.Assets {
			if asset == nil {
				continue
			}
			if count >= maxAssets {
				return saved, warnings, nil
			}
			name := textutil.SanitizePathToken(rel.TagName) + "__" + textutil.SanitizePathToken(asset.Name)
			dest := filepath.Join(destDir, name)
			if err := streamAsset(ctx, ram, owner, repo, asset.ID, dest); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s/%s: %v", rel.TagName, asset.Name, err))
				continue
			}
			saved = append(saved, name)
			count++
		}
	}
	return saved, warnings, nil
}

// streamAsset 流式落单个附件;失败时移除半截文件。
func streamAsset(ctx context.Context, ram sdkprov.ReleaseAssetManager, owner, repo string, assetID int64, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := ram.DownloadReleaseAsset(ctx, owner, repo, assetID, f); err != nil {
		_ = f.Close()
		_ = os.Remove(dest)
		return err
	}
	return f.Close()
}
