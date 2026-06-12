package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFrontCoverSuccess(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	caa := NewCoverArt(c)
	caa.baseURL = srv.URL

	data, ct, ok, err := caa.FetchFrontCover(context.Background(), "rel-mbid-1")
	if err != nil {
		t.Fatalf("FetchFrontCover: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	if len(data) != len(png) {
		t.Errorf("got %d bytes, want %d", len(data), len(png))
	}
}

func TestFetchFrontCover404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	caa := NewCoverArt(c)
	caa.baseURL = srv.URL

	_, _, ok, err := caa.FetchFrontCover(context.Background(), "no-cover")
	if err != nil {
		t.Fatalf("FetchFrontCover: %v", err)
	}
	if ok {
		t.Error("expected ok=false on 404 (no cover, not an error)")
	}
}

func TestFetchFrontCoverFollowsRedirect(t *testing.T) {
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpegbytes"))
	}))
	defer image.Close()

	// CAA replies with a 307 to the actual image host, like the real service.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, image.URL, http.StatusTemporaryRedirect)
	}))
	defer redir.Close()

	c, _ := newTestClient(0)
	caa := NewCoverArt(c)
	caa.baseURL = redir.URL

	data, ct, ok, err := caa.FetchFrontCover(context.Background(), "rel")
	if err != nil {
		t.Fatalf("FetchFrontCover: %v", err)
	}
	if !ok || ct != "image/jpeg" || string(data) != "jpegbytes" {
		t.Fatalf("redirect not followed: ok=%v ct=%q data=%q", ok, ct, string(data))
	}
}
