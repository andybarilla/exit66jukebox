// Package external holds thin, rate-limited clients for the read-only public
// APIs used to enrich poorly-tagged tracks: MusicBrainz and the Cover Art
// Archive. Everything is hand-rolled stdlib — no new dependencies — and every
// outbound request goes through a single shared rate limiter (≤1 req/sec) with
// a descriptive User-Agent, as those services require.
package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	maxRetries  = 3
	maxBodySize = 16 << 20 // 16 MiB cap on any single response body.
)

// limiter spaces outbound requests at least minInterval apart. now/sleep are
// injectable so tests drive a fake clock with zero wall-clock delay.
type limiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	nextAllowed time.Time
	now         func() time.Time
	sleep       func(time.Duration)
}

// wait blocks until the next request is permitted, then reserves the following
// slot. With a fake clock, sleep is expected to advance now.
func (l *limiter) wait() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.nextAllowed) {
		l.sleep(l.nextAllowed.Sub(now))
		now = l.now()
	}
	l.nextAllowed = now.Add(l.minInterval)
}

// Client is a rate-limited HTTP client with a descriptive User-Agent.
type Client struct {
	http      *http.Client
	limiter   *limiter
	userAgent string
}

// New builds a Client that spaces requests minInterval apart and identifies
// itself with userAgent.
func New(userAgent string, minInterval time.Duration) *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
		limiter: &limiter{
			minInterval: minInterval,
			now:         time.Now,
			sleep:       time.Sleep,
		},
		userAgent: userAgent,
	}
}

// do issues a rate-limited GET, retrying on 429/5xx up to maxRetries while
// honoring Retry-After. The caller owns the returned body.
func (c *Client) do(ctx context.Context, url string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		c.limiter.wait()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.userAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.backoff(ctx, attempt, 0)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: %s", url, resp.Status)
			if attempt < maxRetries {
				c.backoff(ctx, attempt, retryAfter)
				continue
			}
			return nil, lastErr
		}
		return resp, nil
	}
	return nil, lastErr
}

// getJSON GETs url and decodes the JSON body into v.
func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	resp, err := c.do(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(v)
}

// backoff sleeps before a retry: Retry-After when the server supplied one, else
// exponential (minInterval << attempt). It uses the limiter's clock so tests
// stay deterministic.
func (c *Client) backoff(ctx context.Context, attempt int, retryAfter time.Duration) {
	d := retryAfter
	if d <= 0 {
		d = c.limiter.minInterval * time.Duration(1<<attempt)
	}
	if d <= 0 {
		return
	}
	c.limiter.sleep(d)
}

// parseRetryAfter reads a Retry-After header expressed as delay-seconds. HTTP
// dates are not honored (the services use seconds).
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
