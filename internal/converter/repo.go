package converter

import (
	"github.com/yi-nology/git-ferry-core/model"
	repomodel "github.com/yi-nology/git-ferry/biz/model/repo"
)

// repoInfoSpec:core Repo → DTO RepoInfo 的搬运契约。
var repoInfoSpec = fieldSpec{
	layout: layoutRFC3339,
}

// ToRepoInfo 把 core 的 Repo 转成对外的 RepoInfo。
func ToRepoInfo(r *model.Repo) *repomodel.RepoInfo {
	if r == nil {
		return nil
	}
	info := &repomodel.RepoInfo{}
	mapFields(info, r, repoInfoSpec)
	return info
}

func ToRepoInfoList(repos []*model.Repo) []*repomodel.RepoInfo {
	result := make([]*repomodel.RepoInfo, 0, len(repos))
	for _, r := range repos {
		result = append(result, ToRepoInfo(r))
	}
	return result
}
