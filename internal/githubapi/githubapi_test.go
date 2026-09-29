package githubapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsGitHub(t *testing.T) {
	assert.True(t, IsGitHub("github"))
	assert.True(t, IsGitHub("GHE"))
	assert.False(t, IsGitHub("gitlab"))
}

func TestDownloadReleaseAssets_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	saved, warns, err := DownloadReleaseAssets(t.Context(), srv.URL, "tok", "o", "r", t.TempDir(), 10)
	require.NoError(t, err)
	assert.Empty(t, saved)
	assert.Empty(t, warns)
}

func TestBackupGists_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	count, warns, err := BackupGists(t.Context(), srv.URL, "tok", t.TempDir(), 50)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, warns)
}

func TestDownloadReleaseAssets_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, _, err := DownloadReleaseAssets(t.Context(), srv.URL, "bad", "o", "r", t.TempDir(), 10)
	assert.Error(t, err)
}

func TestSetHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com", http.NoBody)
	require.NoError(t, err)
	setHeaders(req, "abc")
	assert.Equal(t, "Bearer abc", req.Header.Get("Authorization"))
	assert.Equal(t, "application/vnd.github+json", req.Header.Get("Accept"))
}
