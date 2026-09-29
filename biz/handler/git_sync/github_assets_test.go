package git_sync

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

func TestIsGitHubPlatform(t *testing.T) {
	assert.True(t, isGitHubPlatform("github"))
	assert.True(t, isGitHubPlatform("GHE"))
	assert.False(t, isGitHubPlatform("gitlab"))
}

func TestDownloadGitHubReleaseAssets_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	plat := &corebridge.Platform{APIURL: srv.URL, Type: "github"}
	saved, warns, err := DownloadGitHubReleaseAssets(t.Context(), plat, "tok", "o", "r", t.TempDir(), 10)
	require.NoError(t, err)
	assert.Empty(t, saved)
	assert.Empty(t, warns)
}

func TestBackupGitHubGists_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	plat := &corebridge.Platform{APIURL: srv.URL, Type: "github"}
	count, warns, err := BackupGitHubGists(t.Context(), plat, "tok", t.TempDir(), 50)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, warns)
}

func TestDownloadGitHubReleaseAssets_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	plat := &corebridge.Platform{APIURL: srv.URL, Type: "github"}
	_, _, err := DownloadGitHubReleaseAssets(t.Context(), plat, "bad", "o", "r", t.TempDir(), 10)
	assert.Error(t, err)
}

func TestSetGitHubHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com", http.NoBody)
	require.NoError(t, err)
	setGitHubHeaders(req, "abc")
	assert.Equal(t, "Bearer abc", req.Header.Get("Authorization"))
	assert.Equal(t, "application/vnd.github+json", req.Header.Get("Accept"))
}
