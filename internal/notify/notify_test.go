package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyRun_NtfyAndGotify(t *testing.T) {
	var ntfyHits, gotifyHits int32
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ntfyHits, 1)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("Title"))
		w.WriteHeader(200)
	}))
	defer ntfy.Close()
	gotify := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gotifyHits, 1)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Contains(t, body["title"], "failed")
		w.WriteHeader(200)
	}))
	defer gotify.Close()

	n := New(&Config{
		Ntfy:   []NtfyConfig{{URL: ntfy.URL, Token: "tok"}},
		Gotify: []GotifyConfig{{URL: gotify.URL, Token: "gt"}},
	})
	n.NotifyRun(&RunEvent{TaskKey: "t1", TaskName: "T1", Status: "failed", Error: "boom"})
	// 异步发送,等待短暂
	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&ntfyHits), int32(1))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&gotifyHits), int32(1))
}

func TestNotifyRun_OnlyFailSkipsSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	n := New(&Config{OnlyFail: true, Ntfy: []NtfyConfig{{URL: srv.URL}}})
	n.NotifyRun(&RunEvent{Status: "success"})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits))
}

func TestWantEvent(t *testing.T) {
	assert.True(t, wantEvent(nil, "success"))
	assert.True(t, wantEvent([]string{"failed"}, "failed"))
	assert.False(t, wantEvent([]string{"failed"}, "success"))
}

func TestHeartbeat_FailThenSuccess(t *testing.T) {
	var fail, ok int32
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fail, 1)
		assert.Contains(t, r.URL.Path, "/fail")
		w.WriteHeader(200)
	}))
	defer failSrv.Close()
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ok, 1)
		w.WriteHeader(200)
	}))
	defer okSrv.Close()

	h := NewHeartbeat(&HeartbeatConfig{
		SuccessURLs: []string{okSrv.URL + "/ping"},
		FailURLs:    []string{failSrv.URL + "/ping"},
	})
	h.MarkResult(true)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&fail), int32(1))
	// 失败悬挂期间成功不打成功分路
	before := atomic.LoadInt32(&ok)
	h.MarkResult(false)
	assert.Equal(t, before, atomic.LoadInt32(&ok))
}

func TestSendWebhook_Signed(t *testing.T) {
	var gotBody []byte
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-GitFerry-Signature")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = buf
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n := New(&Config{Webhook: &WebhookConfig{
		FailURL:      srv.URL,
		SharedSecret: "s3cret",
	}})
	n.pushAll(context.Background(), &RunEvent{
		TaskKey: "t1", Status: "failed", Error: "boom",
		EndAt: time.Now(),
	})
	require.NotEmpty(t, gotSig)
	assert.True(t, strings.HasPrefix(gotSig, "sha256="))
	// 签名可复验
	assert.Equal(t, signBody("s3cret", gotBody), gotSig)
}
