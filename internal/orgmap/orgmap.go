// Package orgmap 实现仓库 owner 映射策略（对标 gitea-mirror org_mapping）。
//
// preserve: src_owner/repo → dst_owner/repo（默认）
// single:   全部 → target_org/repo
// flat:     全部 → target_user/repo（个人命名空间）
// mixed:    个人仓→flat；组织仓→preserve 到对应 org
package orgmap

import (
	"fmt"
	"strings"
)

// Policy 映射策略。
type Policy string

const (
	PolicyPreserve Policy = "preserve"
	PolicySingle   Policy = "single"
	PolicyFlat     Policy = "flat"
	PolicyMixed    Policy = "mixed"
)

// ParsePolicy 解析策略名；空串视为 preserve。
func ParsePolicy(s string) (Policy, error) {
	switch Policy(strings.ToLower(strings.TrimSpace(s))) {
	case "", PolicyPreserve:
		return PolicyPreserve, nil
	case PolicySingle:
		return PolicySingle, nil
	case PolicyFlat:
		return PolicyFlat, nil
	case PolicyMixed:
		return PolicyMixed, nil
	default:
		return "", fmt.Errorf("unknown org_mapping %q (preserve|single|flat|mixed)", s)
	}
}

// Input 单仓映射输入。
type Input struct {
	// SourceKey 形如 platform/owner/repo 或 owner/repo
	SourceKey string
	// SourceOwner 源 owner（org 或 user）
	SourceOwner string
	SourceRepo  string
	// IsPersonal 源为个人命名空间（mixed 策略用）
	IsPersonal bool
	// TargetNamespace single/flat 的落点 owner/org
	TargetNamespace string
	// TargetPlatform 目标平台 key 前缀（可选，用于拼 TargetKey）
	TargetPlatform string
}

// Output 映射结果。
type Output struct {
	TargetOwner string
	TargetRepo  string
	// TargetKey 形如 <platform>/<owner>/<repo>（有 TargetPlatform 时）
	TargetKey string
}

// Map 按策略计算目标 owner/repo。
func Map(p Policy, in *Input) (Output, error) {
	owner, repo := in.SourceOwner, in.SourceRepo
	if owner == "" || repo == "" {
		o, r, err := splitOwnerRepo(in.SourceKey)
		if err != nil {
			return Output{}, err
		}
		owner, repo = o, r
	}

	out := Output{TargetRepo: repo}
	switch p {
	case PolicyPreserve, "":
		out.TargetOwner = owner
	case PolicySingle:
		if in.TargetNamespace == "" {
			return Output{}, fmt.Errorf("org_mapping=single requires target_org")
		}
		out.TargetOwner = in.TargetNamespace
	case PolicyFlat:
		if in.TargetNamespace == "" {
			return Output{}, fmt.Errorf("org_mapping=flat requires target_user")
		}
		out.TargetOwner = in.TargetNamespace
	case PolicyMixed:
		if in.IsPersonal {
			if in.TargetNamespace == "" {
				return Output{}, fmt.Errorf("org_mapping=mixed (personal) requires target_user")
			}
			out.TargetOwner = in.TargetNamespace
		} else {
			out.TargetOwner = owner
		}
	default:
		return Output{}, fmt.Errorf("unknown org_mapping %q", p)
	}

	if in.TargetPlatform != "" {
		out.TargetKey = in.TargetPlatform + "/" + out.TargetOwner + "/" + out.TargetRepo
	}
	return out, nil
}

func splitOwnerRepo(key string) (string, string, error) {
	key = strings.Trim(strings.TrimSpace(key), "/")
	parts := strings.Split(key, "/")
	// 支持 platform/owner/repo 或 owner/repo
	if len(parts) == 3 {
		return parts[1], parts[2], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("cannot parse owner/repo from %q", key)
}
