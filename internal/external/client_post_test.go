package external

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPostJSONSendsBodyAndHeaders(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	var out struct {
		Status string `json:"status"`
	}
	err := c.postJSON(context.Background(), srv.URL,
		map[string]string{"Authorization": "Token abc"},
		map[string]any{"hello": "world"}, &out)
	if err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if gotAuth != "Token abc" {
		t.Errorf("Authorization = %q, want Token abc", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["hello"] != "world" {
		t.Errorf("body = %+v, want hello=world", gotBody)
	}
}

// On a 5xx the body must be re-sent on retry, not consumed once — the factory
// design exists for exactly this.
func TestPostJSONRetriesResendBody(t *testing.T) {
	var calls int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	if err := c.postJSON(context.Background(), srv.URL, nil, map[string]any{"k": "v"}, nil); err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (500 then 200)", calls)
	}
	if bodies[0] != bodies[1] || bodies[1] == "" {
		t.Fatalf("retry resent a different/empty body: %q vs %q", bodies[0], bodies[1])
	}
}

func TestPostJSONNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	if err := c.postJSON(context.Background(), srv.URL, nil, map[string]any{}, nil); err == nil {
		t.Fatal("expected error on 400")
	}
}
