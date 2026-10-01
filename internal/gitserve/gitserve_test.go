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
	} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code == http.StatusOK {
			t.Fatalf("%s: want non-200", p)
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
