package git_sync

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

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// githubReleaseAsset GitHub Release 附件元数据(SDK ReleaseInfo 不含 assets,这里直接打 API)。
type githubReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type githubReleaseWithAssets struct {
	TagName string               `json:"tag_name"`
	Name    string               `json:"name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

// DownloadGitHubReleaseAssets 下载仓库全部 Release 附件到 destDir。
// 仅支持 GitHub/GHES(REST API);返回成功/失败清单。
// 这是 git bundle 的盲区:release 二进制不在 git 对象里。
func DownloadGitHubReleaseAssets(ctx context.Context, plat *corebridge.Platform, token, owner, repo, destDir string, maxAssets int) (saved, warnings []string, err error) {
	if maxAssets <= 0 {
		maxAssets = 50
	}
	apiBase := strings.TrimRight(plat.APIURL, "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	client := &http.Client{Timeout: 60 * time.Second}

	listURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", apiBase, owner, repo)
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, listURL, http.NoBody)
	if rerr != nil {
		return nil, nil, rerr
	}
	setGitHubHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("list releases: status %d", resp.StatusCode)
	}
	var releases []githubReleaseWithAssets
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return nil, nil, err
	}
	count := 0
	for _, rel := range releases {
		for _, asset := range rel.Assets {
			if count >= maxAssets {
				return saved, warnings, nil
			}
			name := sanitizePathToken(rel.TagName) + "__" + sanitizePathToken(asset.Name)
			dest := filepath.Join(destDir, name)
			if err := downloadGitHubAsset(ctx, client, token, &asset, dest); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s/%s: %v", rel.TagName, asset.Name, err))
				continue
			}
			saved = append(saved, name)
			count++
		}
	}
	return saved, warnings, nil
}

func downloadGitHubAsset(ctx context.Context, client *http.Client, token string, asset *githubReleaseAsset, dest string) error {
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
	setGitHubHeaders(req, token)
	// API 附件端点需要 octet-stream 才会 302 到真实下载
	if strings.Contains(url, "/releases/assets/") {
		req.Header.Set("Accept", "application/octet-stream")
	}
	resp, err := client.Do(req)
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

// githubGist GitHub Gist 元数据。
type githubGist struct {
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

// BackupGitHubGists 备份当前 token 可见的 gists(含文件内容)。
// gickup 有 starred/gists 附带备份;这里是可检索的本地快照。
func BackupGitHubGists(ctx context.Context, plat *corebridge.Platform, token, destDir string, maxGists int) (count int, warnings []string, err error) {
	if maxGists <= 0 {
		maxGists = 200
	}
	apiBase := strings.TrimRight(plat.APIURL, "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	client := &http.Client{Timeout: 60 * time.Second}
	url := fmt.Sprintf("%s/gists?per_page=100", apiBase)
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if rerr != nil {
		return 0, nil, rerr
	}
	setGitHubHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return 0, nil, fmt.Errorf("list gists: status %d", resp.StatusCode)
	}
	var gists []githubGist
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
		name := sanitizePathToken(g.ID) + ".json"
		if werr := os.WriteFile(filepath.Join(destDir, name), data, 0o600); werr != nil {
			warnings = append(warnings, g.ID+": write")
			continue
		}
		count++
	}
	return count, warnings, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
