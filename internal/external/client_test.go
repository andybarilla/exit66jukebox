package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock advances only when sleep is called, so rate-limiter tests run with
// zero wall-clock delay yet still assert exact spacing.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time        { return f.t }
func (f *fakeClock) sleep(d time.Duration) { f.t = f.t.Add(d) }

func newTestClient(minInterval time.Duration) (*Client, *fakeClock) {
	c := New("test-agent", minInterval)
	clk := &fakeClock{t: time.Unix(0, 0)}
	c.limiter.now = clk.now
	c.limiter.sleep = clk.sleep
	return c, clk
}

func TestGetJSONDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Errorf("User-Agent = %q, want test-agent", got)
		}
		w.Write([]byte(`{"name":"hi","n":7}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	var out struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := c.getJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if out.Name != "hi" || out.N != 7 {
		t.Fatalf("decoded %+v", out)
	}
}

func TestRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c, clk := newTestClient(time.Second)
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.getJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok=true, got %+v", out)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (429 then 200), got %d", calls)
	}
	// The 2s Retry-After backoff subsumes the 1s inter-request spacing, so the
	// fake clock advances by 2s total before the retry succeeds.
	if got := clk.t.Sub(time.Unix(0, 0)); got != 2*time.Second {
		t.Fatalf("clock advanced %v, want 2s (Retry-After subsumes spacing)", got)
	}
}

func TestRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	var out map[string]any
	if err := c.getJSON(context.Background(), srv.URL, &out); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestLimiterSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, clk := newTestClient(time.Second)
	var out map[string]any
	for i := 0; i < 3; i++ {
		if err := c.getJSON(context.Background(), srv.URL, &out); err != nil {
			t.Fatalf("getJSON %d: %v", i, err)
		}
	}
	// First request is immediate; the next two each wait one interval.
	if got := clk.t.Sub(time.Unix(0, 0)); got != 2*time.Second {
		t.Fatalf("clock advanced %v after 3 requests, want 2s", got)
	}
}
