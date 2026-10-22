package gitserve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	h := Handler(Options{BasePath: root})
	for _, p := range []string{
		"/git/../etc/passwd",
		"/git/foo",
		"/git/",
		// Windows 风格分隔符:迁移前 path.Clean 不把它当分隔符,口径与 Linux 不一致
		`/git/..\..\etc\passwd`,
		// 绝对路径与盘符
		"/git//etc/passwd",
		`/git/C:\windows\system32`,
		// UNC
		`/git//host/share/x.git`,
		// 编码后的穿越(%2e%2e%2f 由 net/http 解码成 ../)
		"/git/%2e%2e%2fetc/passwd",
	} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code == http.StatusOK {
			t.Fatalf("%s: want non-200", p)
		}
	}
}

// TestHandler_AllowsLegitimateNames 合法的仓库路径(含 ".." 字样的名字与重复斜杠)
// 不能被误拒——迁移前用 strings.Contains(p, "..") 子串判定,会误杀这类名字。
func TestHandler_AllowsLegitimateNames(t *testing.T) {
	root := t.TempDir()
	h := Handler(Options{BasePath: root})
	for _, p := range []string{
		"/git/github/o/foo..bar.git/info/refs",
		"/git//github/o/r.git/info/refs",
	} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		rr := httptest.NewRecorder()
		h(rr, req)
		// 仓库不存在 → 404;关键是不能是 400/500 之类的"路径非法"反应
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s: code=%d want 404(路径应被判为合法,只是仓库不存在)", p, rr.Code)
		}
	}
}

func TestHandler_MissingRepo404(t *testing.T) {
	root := t.TempDir()
	h := Handler(Options{BasePath: root})
	req := httptest.NewRequest(http.MethodGet, "/git/github/o/r.git/info/refs?service=git-upload-pack", http.NoBody)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestHandler_EmptyBase503(t *testing.T) {
	h := Handler(Options{})
	req := httptest.NewRequest(http.MethodGet, "/git/a/b.git/info/refs", http.NoBody)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestCloneURL(t *testing.T) {
	got := CloneURL("http://g:8890", "github/o/r.git")
	want := "http://g:8890/git/github/o/r.git"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	// ensure path is under root when repo exists (info/refs empty is ok)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "github", "o", "r.git"), 0o750); err != nil {
		t.Fatal(err)
	}
}
