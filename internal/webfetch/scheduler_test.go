package webfetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(fn roundTripFunc) *Client {
	return &Client{http: &http.Client{Transport: fn}}
}

func response(req *http.Request, status int, headers map[string]string, body string) *http.Response {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestQueuedCache(t *testing.T) {
	var calls atomic.Int32
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(r, http.StatusOK, map[string]string{
			"Content-Type": "application/json",
			"ETag":         `"abc"`,
		}, `{"ok":true}`), nil
	})
	zero := 0
	ttl := 1000
	retry := 0
	req := QueueRequest{URL: "https://example.com/cache", MinIntervalMs: &zero, Retry: &retry, CacheTTLms: &ttl}
	a, err := c.FetchQueued(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.FetchQueued(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.Cached || !b.Cached || calls.Load() != 1 {
		t.Fatalf("a.cached=%v b.cached=%v calls=%d", a.Cached, b.Cached, calls.Load())
	}
}

func TestBatchKeepsOrder(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		raw, _ := json.Marshal(map[string]string{"path": r.URL.Path})
		return response(r, http.StatusOK, map[string]string{"Content-Type": "application/json"}, string(raw)), nil
	})
	zero := 0
	retry := 0
	got, err := c.FetchBatch(context.Background(), BatchRequest{
		Requests:    []QueueRequest{{URL: "https://example.com/a"}, {URL: "https://example.com/b"}, {URL: "https://example.com/c"}},
		Concurrency: 3, MinIntervalMs: &zero, Retry: &retry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 3 || got.Completed != 3 {
		t.Fatalf("unexpected batch: %+v", got)
	}
	for i, item := range got.Results {
		if item.Index != i || !item.OK {
			t.Fatalf("bad result %d: %+v", i, item)
		}
	}
}

func TestQueueSpacing(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		return response(r, http.StatusNoContent, nil, ""), nil
	})
	interval := 35
	retry := 0
	start := time.Now()
	_, err := c.FetchQueued(context.Background(), QueueRequest{URL: "https://example.com/spacing", Group: "spacing", MinIntervalMs: &interval, Retry: &retry})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.FetchQueued(context.Background(), QueueRequest{URL: "https://example.com/spacing", Group: "spacing", MinIntervalMs: &interval, Retry: &retry})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatalf("queue did not space requests: %v", time.Since(start))
	}
}
