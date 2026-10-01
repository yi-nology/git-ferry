package githubapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRateLimit_RetryAfter(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}}
	resp.Header.Set("Retry-After", "7")
	reason, d := parseRateLimit(resp, time.Second, time.Minute, 1)
	if reason != "retry-after=7" || d != 7*time.Second {
		t.Fatalf("reason=%s d=%v", reason, d)
	}
}

func TestParseRateLimit_Exponential(t *testing.T) {
	resp := &http.Response{StatusCode: 403, Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Remaining", "0")
	_, d1 := parseRateLimit(resp, time.Second, time.Minute, 1)
	_, d3 := parseRateLimit(resp, time.Second, time.Minute, 3)
	if d1 != time.Second {
		t.Fatalf("attempt1=%v", d1)
	}
	if d3 != 4*time.Second {
		t.Fatalf("attempt3=%v", d3)
	}
}

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		status int
		hdr    map[string]string
		want   bool
	}{
		{429, nil, true},
		{403, map[string]string{"X-RateLimit-Remaining": "0"}, true},
		{403, map[string]string{"Retry-After": "3"}, true},
		{403, map[string]string{}, false},
		{200, nil, false},
	}
	for i, c := range cases {
		resp := &http.Response{StatusCode: c.status, Header: http.Header{}}
		for k, v := range c.hdr {
			resp.Header.Set(k, v)
		}
		if got := isRateLimited(resp); got != c.want {
			t.Fatalf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestThrottler_Do_RetriesThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var events []RateLimitEvent
	th := NewThrottler()
	th.BaseDelay = time.Millisecond
	th.MaxDelay = 10 * time.Millisecond
	th.Sleep = func(context.Context, time.Duration) error { return nil }
	th.OnEvent = func(ev RateLimitEvent) { events = append(events, ev) }

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
	resp, err := th.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls=%d", calls)
	}
	if len(events) != 1 || events[0].Attempt != 1 {
		t.Fatalf("events=%+v", events)
	}
}

func TestThrottler_Do_GivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
	}))
	defer srv.Close()

	th := NewThrottler()
	th.MaxAttempts = 2
	th.BaseDelay = time.Millisecond
	th.Sleep = func(context.Context, time.Duration) error { return nil }

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
	if _, err := th.Do(context.Background(), req); err == nil {
		t.Fatal("want rate limit error")
	}
}
