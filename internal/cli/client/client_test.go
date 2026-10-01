package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDo_StdShell(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer-less", r.Header.Get("X-API-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"key":"t1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "Bearer-less")
	env, err := c.Get("/api/v1/x", nil)
	require.NoError(t, err)
	require.True(t, env.OK)
}

func TestDo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"code":404,"message":"nope"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	env, err := c.Get("/missing", nil)
	require.NoError(t, err)
	assert.False(t, env.OK)
	assert.Equal(t, 404, env.Error.Code)
}

func TestPaginateAll(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		// 第 1 页 2 条，第 2 页 1 条（< limit=2 结束）
		body := `{"list":[{"id":1},{"id":2}]}`
		if page >= 2 {
			body = `{"list":[{"id":3}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	q := url.Values{"per_page": {"2"}}
	items, err := c.PaginateAll("/api/v1/repos", q)
	require.NoError(t, err)
	assert.Len(t, items, 3)

	var first map[string]any
	require.NoError(t, json.Unmarshal(items[0], &first))
	assert.EqualValues(t, 1, first["id"])
}

func TestPaginateAll_StopsOnEmpty(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{"tasks":[{"key":"a"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	items, err := c.PaginateAll("/api/v1/sync/tasks", nil)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}
