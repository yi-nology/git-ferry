// Package gitserve 只读 Git Smart HTTP：局域网/灾备场景直接 `git clone`
// 冷备或 workdir 中的 bare/mirror 仓库，不必先推到另一 forge。
package gitserve

import (
	"net/http"
	"net/http/cgi" //nolint:gosec // G504: 调用本机 git http-backend；仅监听内网/灾备端口，不代理外部 CGI
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Options 服务配置。
type Options struct {
	// BasePath bare 仓库根目录（形如 <root>/<host>/<owner>/<repo>.git）
	BasePath string
	// PublicRead true 时跳过调用方鉴权（路由层控制）
	PublicRead bool
}

// Handler 返回 /git/* 的 http.HandlerFunc（映射到 git http-backend）。
// 路径形如 /git/github/octocat/hello.git/info/refs
func Handler(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opt.BasePath == "" {
			http.Error(w, "git_serve.base_path not configured", http.StatusServiceUnavailable)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/git/")
		p = path.Clean("/" + p) // 防目录穿越
		p = strings.TrimPrefix(p, "/")
		if p == "" || strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		// 仅允许 .git 后缀仓库路径
		if !strings.Contains(p, ".git") {
			http.NotFound(w, r)
			return
		}
		absRoot, err := filepath.Abs(opt.BasePath)
		if err != nil {
			http.Error(w, "invalid base path", http.StatusInternalServerError)
			return
		}
		full := filepath.Join(absRoot, p)
		// 确保不越出 BasePath
		if !strings.HasPrefix(full, absRoot+string(os.PathSeparator)) && full != absRoot {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(full); err != nil {
			http.NotFound(w, r)
			return
		}

		gitPath, gerr := exec.LookPath("git")
		if gerr != nil {
			http.Error(w, "git not installed on server", http.StatusServiceUnavailable)
			return
		}
		h := &cgi.Handler{
			Path: gitPath,
			Args: []string{"http-backend"},
			Dir:  absRoot,
			Env: []string{
				"GIT_PROJECT_ROOT=" + absRoot,
				"GIT_HTTP_EXPORT_ALL=1",
				"PATH=" + os.Getenv("PATH"),
			},
		}
		// http-backend 用 PATH_INFO 定位仓库
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + p
		r2.RequestURI = "/" + p
		h.ServeHTTP(w, r2)
	}
}

// CloneURL 拼出对外 clone 地址（供 CLI/API 返回）。
func CloneURL(publicBase, repoPath string) string {
	return strings.TrimRight(publicBase, "/") + "/git/" + strings.TrimPrefix(repoPath, "/")
}
