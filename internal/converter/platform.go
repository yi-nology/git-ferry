package converter

import (
	"time"

	platformmodel "github.com/yi-nology/git-ferry/biz/model/platform"
	"github.com/yi-nology/git-sync-core/model"
)

// ToPlatformInfo 将平台模型转为 IDL 生成的对外结构 platformmodel.PlatformInfo。
// 日期按 RFC3339 格式化，与 model.Platform（time.Time 默认 JSON）保持一致。
func ToPlatformInfo(p *model.Platform) *platformmodel.PlatformInfo {
	if p == nil {
		return nil
	}
	lastTestAt := ""
	if p.LastTestAt != nil {
		lastTestAt = p.LastTestAt.Format(time.RFC3339)
	}
	return &platformmodel.PlatformInfo{
		ID:             SafeUintToInt64(p.ID),
		Key:            p.Key,
		Name:           p.Name,
		Type:           p.Type,
		InstanceUrl:    p.InstanceURL,
		ApiUrl:         p.APIURL,
		SkipTlsVerify:  p.SkipTLSVerify,
		CaCertPath:     p.CACertPath,
		ProxyUrl:       p.ProxyURL,
		IsDefault:      p.IsDefault,
		Status:         p.Status,
		LastTestResult: p.LastTestResult,
		RepoCount:      SafeIntToInt32(p.RepoCount),
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
		LastTestAt:     lastTestAt,
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
		HasToken:       p.AccessToken != "",
	}
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
	if req.ProxyUrl != "" {
		p.ProxyURL = req.ProxyUrl
	}
	if req.IsDefault != nil {
		p.IsDefault = *req.IsDefault
	}
}
