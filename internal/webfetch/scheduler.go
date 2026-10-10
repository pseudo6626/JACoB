package webfetch

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultQueueIntervalMs = 250
	maxQueueIntervalMs     = 60_000
	maxCacheTTLms          = 24 * 60 * 60 * 1000
	maxQueuedRetries       = 5
	maxBatchRequests       = 64
	maxBatchConcurrency    = 8
	maxSchedulerCacheItems = 128
)

type QueueRequest struct {
	URL           string            `json:"url"`
	Method        string            `json:"method,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          string            `json:"body,omitempty"`
	Group         string            `json:"group,omitempty"`
	MinIntervalMs *int              `json:"minIntervalMs,omitempty"`
	Retry         *int              `json:"retry,omitempty"`
	CacheTTLms    *int              `json:"cacheTtlMs,omitempty"`
}

type QueuedResponse struct {
	Response
	Cached      bool `json:"cached"`
	Revalidated bool `json:"revalidated"`
	Attempts    int  `json:"attempts"`
}

type BatchRequest struct {
	Requests      []QueueRequest `json:"requests"`
	Group         string         `json:"group,omitempty"`
	Concurrency   int            `json:"concurrency,omitempty"`
	MinIntervalMs *int           `json:"minIntervalMs,omitempty"`
	PauseEvery    int            `json:"pauseEvery,omitempty"`
	PauseMs       int            `json:"pauseMs,omitempty"`
	Retry         *int           `json:"retry,omitempty"`
	CacheTTLms    *int           `json:"cacheTtlMs,omitempty"`
}

type BatchItemResult struct {
	Index    int             `json:"index"`
	OK       bool            `json:"ok"`
	Response *QueuedResponse `json:"response,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type BatchResult struct {
	Results   []BatchItemResult `json:"results"`
	Completed int               `json:"completed"`
	Failed    int               `json:"failed"`
}

type schedulerState struct {
	mu     sync.Mutex
	groups map[string]*queueGroup
	cache  map[string]cacheEntry
}

type queueGroup struct {
	mu   sync.Mutex
	next time.Time
}

type cacheEntry struct {
	response     Response
	expires      time.Time
	etag         string
	lastModified string
	stored       time.Time
}

var schedulerStates sync.Map // map[*Client]*schedulerState

func stateFor(c *Client) *schedulerState {
	if c == nil {
		return &schedulerState{groups: map[string]*queueGroup{}, cache: map[string]cacheEntry{}}
	}
	if v, ok := schedulerStates.Load(c); ok {
		return v.(*schedulerState)
	}
	s := &schedulerState{groups: map[string]*queueGroup{}, cache: map[string]cacheEntry{}}
	actual, _ := schedulerStates.LoadOrStore(c, s)
	return actual.(*schedulerState)
}

func (c *Client) FetchQueued(ctx context.Context, in QueueRequest) (QueuedResponse, error) {
	if c == nil {
		return QueuedResponse{}, fmt.Errorf("web fetch client is unavailable")
	}
	base := Request{URL: in.URL, Method: in.Method, Headers: cloneHeaders(in.Headers), Body: in.Body}
	interval := optionInt(in.MinIntervalMs, defaultQueueIntervalMs, 0, maxQueueIntervalMs)
	retries := optionInt(in.Retry, 2, 0, maxQueuedRetries)
	ttlMs := optionInt(in.CacheTTLms, 0, 0, maxCacheTTLms)
	group := normalizeQueueGroup(in.Group, in.URL)
	state := stateFor(c)
	key := cacheKey(base)

	var stale *cacheEntry
	if strings.EqualFold(strings.TrimSpace(base.Method), "GET") || strings.TrimSpace(base.Method) == "" {
		if ttlMs > 0 {
			if ent, ok := cacheLookup(state, key); ok {
				if time.Now().Before(ent.expires) {
					return QueuedResponse{Response: ent.response, Cached: true, Attempts: 0}, nil
				}
				copyEnt := ent
				stale = &copyEnt
				if base.Headers == nil {
					base.Headers = map[string]string{}
				}
				if ent.etag != "" {
					base.Headers["If-None-Match"] = ent.etag
				}
				if ent.lastModified != "" {
					base.Headers["If-Modified-Since"] = ent.lastModified
				}
			}
		}
	}

	var last Response
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := waitForQueueSlot(ctx, state, group, time.Duration(interval)*time.Millisecond); err != nil {
			return QueuedResponse{}, err
		}
		last, lastErr = c.Fetch(ctx, base)
		if lastErr == nil && last.Status == 304 && stale != nil {
			cacheRefresh(state, key, *stale, time.Duration(ttlMs)*time.Millisecond)
			return QueuedResponse{Response: stale.response, Cached: true, Revalidated: true, Attempts: attempt + 1}, nil
		}
		if lastErr == nil && !retryableStatus(last.Status) {
			if ttlMs > 0 && isCacheable(base, last) {
				cacheStore(state, key, last, time.Duration(ttlMs)*time.Millisecond)
			}
			return QueuedResponse{Response: last, Attempts: attempt + 1}, nil
		}
		if attempt == retries {
			break
		}
		if err := sleepContext(ctx, retryDelay(attempt)); err != nil {
			return QueuedResponse{}, err
		}
	}
	if lastErr != nil {
		return QueuedResponse{}, lastErr
	}
	return QueuedResponse{Response: last, Attempts: retries + 1}, nil
}

func (c *Client) FetchBatch(ctx context.Context, in BatchRequest) (BatchResult, error) {
	if len(in.Requests) == 0 {
		return BatchResult{Results: []BatchItemResult{}}, nil
	}
	if len(in.Requests) > maxBatchRequests {
		return BatchResult{}, fmt.Errorf("batch contains %d requests; maximum is %d", len(in.Requests), maxBatchRequests)
	}
	concurrency := in.Concurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	if concurrency > maxBatchConcurrency {
		concurrency = maxBatchConcurrency
	}
	pauseEvery := in.PauseEvery
	if pauseEvery <= 0 || pauseEvery > len(in.Requests) {
		pauseEvery = len(in.Requests)
	}
	pauseMs := in.PauseMs
	if pauseMs < 0 {
		pauseMs = 0
	}
	if pauseMs > 10*60*1000 {
		pauseMs = 10 * 60 * 1000
	}

	results := make([]BatchItemResult, len(in.Requests))
	for start := 0; start < len(in.Requests); start += pauseEvery {
		end := start + pauseEvery
		if end > len(in.Requests) {
			end = len(in.Requests)
		}
		jobs := make(chan int)
		var wg sync.WaitGroup
		workers := concurrency
		if workers > end-start {
			workers = end - start
		}
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					item := in.Requests[i]
					if strings.TrimSpace(item.Group) == "" {
						item.Group = in.Group
					}
					if item.MinIntervalMs == nil {
						item.MinIntervalMs = in.MinIntervalMs
					}
					if item.Retry == nil {
						item.Retry = in.Retry
					}
					if item.CacheTTLms == nil {
						item.CacheTTLms = in.CacheTTLms
					}
					resp, err := c.FetchQueued(ctx, item)
					if err != nil {
						results[i] = BatchItemResult{Index: i, OK: false, Error: err.Error()}
						continue
					}
					copyResp := resp
					results[i] = BatchItemResult{Index: i, OK: true, Response: &copyResp}
				}
			}()
		}
		for i := start; i < end; i++ {
			select {
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return summarizeBatch(results), ctx.Err()
			case jobs <- i:
			}
		}
		close(jobs)
		wg.Wait()
		if end < len(in.Requests) && pauseMs > 0 {
			if err := sleepContext(ctx, time.Duration(pauseMs)*time.Millisecond); err != nil {
				return summarizeBatch(results), err
			}
		}
	}
	return summarizeBatch(results), nil
}

func summarizeBatch(results []BatchItemResult) BatchResult {
	out := BatchResult{Results: results}
	for _, item := range results {
		if item.OK {
			out.Completed++
		} else if item.Error != "" {
			out.Failed++
		}
	}
	return out
}

func waitForQueueSlot(ctx context.Context, state *schedulerState, group string, interval time.Duration) error {
	state.mu.Lock()
	g := state.groups[group]
	if g == nil {
		g = &queueGroup{}
		state.groups[group] = g
	}
	state.mu.Unlock()

	g.mu.Lock()
	now := time.Now()
	start := now
	if g.next.After(start) {
		start = g.next
	}
	g.next = start.Add(interval)
	g.mu.Unlock()

	wait := time.Until(start)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func normalizeQueueGroup(group, rawURL string) string {
	group = strings.TrimSpace(group)
	if group != "" {
		if len(group) > 96 {
			group = group[:96]
		}
		return strings.ToLower(group)
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && u.Hostname() != "" {
		return "host:" + strings.ToLower(u.Hostname())
	}
	return "default"
}

func cacheKey(in Request) string {
	keys := make([]string, 0, len(in.Headers))
	for k := range in.Headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "if-none-match" || lk == "if-modified-since" {
			continue
		}
		keys = append(keys, lk)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(strings.ToUpper(strings.TrimSpace(in.Method)))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(in.URL))
	b.WriteByte('\n')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(':')
		for hk, hv := range in.Headers {
			if strings.EqualFold(strings.TrimSpace(hk), k) {
				b.WriteString(hv)
				break
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString(in.Body)
	return b.String()
}

func cacheLookup(state *schedulerState, key string) (cacheEntry, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	ent, ok := state.cache[key]
	return ent, ok
}

func cacheStore(state *schedulerState, key string, response Response, ttl time.Duration) {
	now := time.Now()
	ent := cacheEntry{
		response: response, expires: now.Add(ttl), stored: now,
		etag: response.Headers["ETag"], lastModified: response.Headers["Last-Modified"],
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.cache[key] = ent
	if len(state.cache) <= maxSchedulerCacheItems {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, item := range state.cache {
		if oldestKey == "" || item.stored.Before(oldest) {
			oldestKey, oldest = k, item.stored
		}
	}
	if oldestKey != "" {
		delete(state.cache, oldestKey)
	}
}

func cacheRefresh(state *schedulerState, key string, ent cacheEntry, ttl time.Duration) {
	ent.expires = time.Now().Add(ttl)
	ent.stored = time.Now()
	state.mu.Lock()
	state.cache[key] = ent
	state.mu.Unlock()
}

func isCacheable(in Request, response Response) bool {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = "GET"
	}
	if method != "GET" || response.Status < 200 || response.Status >= 300 {
		return false
	}
	return !strings.Contains(strings.ToLower(response.Headers["Cache-Control"]), "no-store")
}

func retryableStatus(status int) bool {
	return status == 408 || status == 425 || status == 429 || status >= 500
}

func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 4 {
		attempt = 4
	}
	return time.Duration(250*(1<<uint(attempt))) * time.Millisecond
}

func optionInt(v *int, def, min, max int) int {
	n := def
	if v != nil {
		n = *v
	}
	if n < min {
		n = min
	}
	if n > max {
		n = max
	}
	return n
}

func cloneHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// StringifyQueueRequest is used by diagnostics/tests when showing effective queue settings.
func StringifyQueueRequest(in QueueRequest) string {
	return normalizeQueueGroup(in.Group, in.URL) + ":" + strconv.Itoa(optionInt(in.MinIntervalMs, defaultQueueIntervalMs, 0, maxQueueIntervalMs))
}
