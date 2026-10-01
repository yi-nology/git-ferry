// Package githubapi 封装 GitHub REST 专属能力(release 附件、gists),
// 与业务 handler 解耦:只依赖 corebridge 模型与标准库 HTTP。
package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// Platform 平台连接信息(githubapi 不依赖 corebridge,降低耦合)。
type Platform struct {
	APIURL string
	Type   string
}

// IsGitHub 判断平台类型是否 GitHub/GHES。
func IsGitHub(platformType string) bool {
	return strings.EqualFold(platformType, "github") || strings.EqualFold(platformType, "ghe")
}

// ReleaseAsset GitHub Release 附件元数据。
type ReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type releaseWithAssets struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

// Gist GitHub Gist 元数据。
type Gist struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Public      bool      `json:"public"`
	HTMLURL     string    `json:"html_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Files       map[string]struct {
		Filename string `json:"filename"`
		Language string `json:"language"`
		RawURL   string `json:"raw_url"`
		Size     int64  `json:"size"`
		Content  string `json:"content"`
	} `json:"files"`
}

// DownloadReleaseAssets 下载仓库 Release 附件到 destDir。
func DownloadReleaseAssets(ctx context.Context, apiURL, token, owner, repo, destDir string, maxAssets int) (saved, warnings []string, err error) {
	if maxAssets <= 0 {
		maxAssets = 50
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	th := NewThrottler()
	client := th.Client

	listURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", base, owner, repo)
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, listURL, http.NoBody)
	if rerr != nil {
		return nil, nil, rerr
	}
	setHeaders(req, token)
	resp, err := th.Do(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("list releases: status %d", resp.StatusCode)
	}
	var releases []releaseWithAssets
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return nil, nil, err
	}
	count := 0
	for _, rel := range releases {
		for i := range rel.Assets {
			if count >= maxAssets {
				return saved, warnings, nil
			}
			asset := &rel.Assets[i]
			name := textutil.SanitizePathToken(rel.TagName) + "__" + textutil.SanitizePathToken(asset.Name)
			dest := filepath.Join(destDir, name)
			if err := downloadAsset(ctx, client, token, asset, dest); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s/%s: %v", rel.TagName, asset.Name, err))
				continue
			}
			saved = append(saved, name)
			count++
		}
	}
	return saved, warnings, nil
}

func downloadAsset(ctx context.Context, client *http.Client, token string, asset *ReleaseAsset, dest string) error {
	url := asset.URL
	if url == "" {
		url = asset.BrowserDownloadURL
	}
	if url == "" {
		return fmt.Errorf("no download url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	setHeaders(req, token)
	if strings.Contains(url, "/releases/assets/") {
		req.Header.Set("Accept", "application/octet-stream")
	}
	th := NewThrottler()
	th.Client = client
	resp, err := th.Do(ctx, req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

// BackupGists 备份 token 可见的 gists 到 destDir。
func BackupGists(ctx context.Context, apiURL, token, destDir string, maxGists int) (count int, warnings []string, err error) {
	if maxGists <= 0 {
		maxGists = 200
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	th := NewThrottler()
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, base+"/gists?per_page=100", http.NoBody)
	if rerr != nil {
		return 0, nil, rerr
	}
	setHeaders(req, token)
	resp, err := th.Do(ctx, req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return 0, nil, fmt.Errorf("list gists: status %d", resp.StatusCode)
	}
	var gists []Gist
	if err := json.NewDecoder(resp.Body).Decode(&gists); err != nil {
		return 0, nil, err
	}
	if len(gists) > maxGists {
		gists = gists[:maxGists]
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return 0, nil, err
	}
	for _, g := range gists {
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

func setHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
