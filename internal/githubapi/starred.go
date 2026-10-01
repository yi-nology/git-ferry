package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// StarredRepo GitHub starred 仓库摘要。
type StarredRepo struct {
	FullName    string `json:"full_name"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	CloneURL    string `json:"clone_url"`
	HTMLURL     string `json:"html_url"`
	Private     bool   `json:"private"`
	Archived    bool   `json:"archived"`
	Fork        bool   `json:"fork"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
	Description string `json:"description"`
}

type ghRepoJSON struct {
	FullName    string `json:"full_name"`
	Name        string `json:"name"`
	CloneURL    string `json:"clone_url"`
	HTMLURL     string `json:"html_url"`
	Private     bool   `json:"private"`
	Archived    bool   `json:"archived"`
	Fork        bool   `json:"fork"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
	Description string `json:"description"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// ListStarred 列出 token 用户的 starred 仓库（GitHub）。
func ListStarred(ctx context.Context, apiURL, token string, max int) ([]StarredRepo, error) {
	if max <= 0 {
		max = 200
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	th := NewThrottler()
	out := []StarredRepo{}
	for page := 1; len(out) < max; page++ {
		u := fmt.Sprintf("%s/user/starred?per_page=100&page=%d", base, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return out, err
		}
		setHeaders(req, token)
		req.Header.Set("Accept", "application/vnd.github.star+json")
		resp, err := th.Do(ctx, req)
		if err != nil {
			return out, err
		}
		var batch []struct {
			Repo ghRepoJSON `json:"repo"`
		}
		// 兼容两种形状：[{repo:{...}}] 或裸 repo 列表
		dec := json.NewDecoder(resp.Body)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			_ = resp.Body.Close()
			return out, err
		}
		_ = resp.Body.Close()
		if err := json.Unmarshal(raw, &batch); err == nil && len(batch) > 0 && batch[0].Repo.FullName != "" {
			for i := range batch {
				out = append(out, toStarred(&batch[i].Repo))
			}
		} else {
			var repos []ghRepoJSON
			if err := json.Unmarshal(raw, &repos); err != nil {
				return out, err
			}
			for i := range repos {
				out = append(out, toStarred(&repos[i]))
			}
		}
		if len(batch) < 100 {
			// 页不足一页则结束
			var repos []ghRepoJSON
			_ = json.Unmarshal(raw, &repos)
			if len(batch) < 100 && len(repos) < 100 {
				break
			}
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// ListPublicOrgRepos 匿名列组织公开仓库（无需 token 也可）。
func ListPublicOrgRepos(ctx context.Context, apiURL, token, org string, max int) ([]StarredRepo, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}
	if max <= 0 {
		max = 200
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	th := NewThrottler()
	out := []StarredRepo{}
	for page := 1; len(out) < max; page++ {
		u := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&page=%d&type=public", base, url.PathEscape(org), page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return out, err
		}
		setHeaders(req, token)
		resp, err := th.Do(ctx, req)
		if err != nil {
			return out, err
		}
		var repos []ghRepoJSON
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			_ = resp.Body.Close()
			return out, err
		}
		_ = resp.Body.Close()
		for i := range repos {
			out = append(out, toStarred(&repos[i]))
		}
		if len(repos) < 100 {
			break
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

func toStarred(r *ghRepoJSON) StarredRepo {
	owner := r.Owner.Login
	if owner == "" {
		if i := strings.Index(r.FullName, "/"); i > 0 {
			owner = r.FullName[:i]
		}
	}
	return StarredRepo{
		FullName:    r.FullName,
		Name:        r.Name,
		Owner:       owner,
		CloneURL:    r.CloneURL,
		HTMLURL:     r.HTMLURL,
		Private:     r.Private,
		Archived:    r.Archived,
		Fork:        r.Fork,
		Stars:       r.Stars,
		Language:    r.Language,
		Description: r.Description,
	}
}
