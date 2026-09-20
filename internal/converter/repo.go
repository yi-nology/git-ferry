package converter

import (
	"time"

	repomodel "github.com/yi-nology/git-ferry/biz/model/repo"
	"github.com/yi-nology/git-ferry-core/model"
)

func ToRepoInfo(r *model.Repo) *repomodel.RepoInfo {
	if r == nil {
		return nil
	}
	return &repomodel.RepoInfo{
		ID:            SafeUintToInt64(r.ID),
		Key:           r.Key,
		Name:          r.Name,
		Platform:      r.Platform,
		PlatformOwner: r.PlatformOwner,
		PlatformRepo:  r.PlatformRepo,
		CloneUrl:      r.CloneURL,
		SshUrl:        r.SSHURL,
		DefaultBranch: r.DefaultBranch,
		Status:        r.Status,
		// 用 RFC3339 与 platform 响应统一,且能被 JS new Date() 可靠解析
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

func ToRepoInfoList(repos []*model.Repo) []*repomodel.RepoInfo {
	result := make([]*repomodel.RepoInfo, 0, len(repos))
	for _, r := range repos {
		result = append(result, ToRepoInfo(r))
	}
	return result
}
