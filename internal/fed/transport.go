package fed

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"
)

// SessionClient returns an http.Client whose connections are streams opened on
// the given yamux session. The request URL's host is ignored — every request
// rides the one session — so callers use any placeholder host. Range headers
// and 206 responses pass through unchanged, which is what makes audio seeking
// work across the relay.
func SessionClient(sess *yamux.Session) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sess.Open()
			},
			// One session multiplexes many streams; disable connection pooling
			// quirks that assume distinct TCP conns.
			DisableKeepAlives:     true,
			MaxIdleConns:          -1,
			IdleConnTimeout:       time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}
