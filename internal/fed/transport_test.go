package fed

import (
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/hashicorp/yamux"
)

func TestHTTPOverSession(t *testing.T) {
	c1, c2 := net.Pipe()
	server, err := yamux.Server(c1, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := yamux.Client(c2, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Serve a handler over the server session.
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	go http.Serve(server, mux)

	// Client makes a request over the client session.
	hc := SessionClient(client)
	resp, err := hc.Get("http://peer/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("expected pong, got %q", body)
	}
}
