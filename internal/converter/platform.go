package converter

import (
	"github.com/yi-nology/git-ferry-core/model"
	platformmodel "github.com/yi-nology/git-ferry/biz/model/platform"
)

// platformInfoSpec:core Platform → DTO PlatformInfo 的搬运契约。
var platformInfoSpec = fieldSpec{
	layout: layoutRFC3339,
	// HasToken / HasGithubApp 是算出来的(secret 只暴露"有没有"),
	// 不是从 core 字段搬来的,声明为派生字段。
	derived: map[string]bool{"hastoken": true, "hasgithubapp": true},
}

// ToPlatformInfo 将平台模型转为 IDL 生成的对外结构 platformmodel.PlatformInfo。
// 日期按 RFC3339 格式化，与 model.Platform（time.Time 默认 JSON）保持一致。
func ToPlatformInfo(p *model.Platform) *platformmodel.PlatformInfo {
	if p == nil {
		return nil
	}
	info := &platformmodel.PlatformInfo{}
	mapFields(info, p, platformInfoSpec)
	// 派生字段:AccessToken / GitHubPrivateKey 是 secret,只暴露"配没配"。
	info.HasToken = p.AccessToken != ""
	info.HasGithubApp = p.GitHubAppID > 0 && p.GitHubInstallationID > 0
	return info
}

// ToPlatformList 批量转换平台。
func ToPlatformList(platforms []*model.Platform) []*platformmodel.PlatformInfo {
	result := make([]*platformmodel.PlatformInfo, 0, len(platforms))
	for _, p := range platforms {
		result = append(result, ToPlatformInfo(p))
	}
	return result
}

// ApplyPlatformUpdate 将更新请求中的非空字段合并进平台模型(空值表示不更新)。
//
// 这里刻意不套 mapFields:它是"更新策略"而不是"字段搬运"——
// 哪些字段允许改本身就是业务口径(例如 Type/Key 不接受更新),
// 且各字段的"未更新"哨兵不统一:字符串用空串、bool 用 nil 指针、
// GitHub App 的 id 用 >0(0 与负数都不更新)。反射搬运表达不了这套语义,
// 保持逐字段更安全。
func ApplyPlatformUpdate(p *model.Platform, req *platformmodel.UpdatePlatformReq) {
	if p == nil || req == nil {
		return
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.InstanceUrl != "" {
		p.InstanceURL = req.InstanceUrl
	}
	if req.ApiUrl != "" {
		p.APIURL = req.ApiUrl
	}
	if req.AccessToken != "" {
		p.AccessToken = req.AccessToken
	}
	if req.SkipTlsVerify != nil {
		p.SkipTLSVerify = *req.SkipTlsVerify
	}
	if req.CaCertPath != "" {
		p.CACertPath = req.CaCertPath
	}
	if req.SSHHostKeyFingerprint != "" {
		p.SSHHostKeyFingerprint = req.SSHHostKeyFingerprint
	}
	if req.SSHKnownHostsPath != "" {
		p.SSHKnownHostsPath = req.SSHKnownHostsPath
	}
	if req.ProxyUrl != "" {
		p.ProxyURL = req.ProxyUrl
	}
	// GitHub App:私钥空=不更新;app/installation id 0=不更新(与 token 同语义)
	if req.GitHubAppID > 0 {
		p.GitHubAppID = req.GitHubAppID
	}
	if req.GitHubInstallationID > 0 {
		p.GitHubInstallationID = req.GitHubInstallationID
	}
	if req.GitHubPrivateKey != "" {
		p.GitHubPrivateKey = req.GitHubPrivateKey
	}
	if req.IsDefault != nil {
		p.IsDefault = *req.IsDefault
	}
}
