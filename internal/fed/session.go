package fed

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/yamux"
)

// Peer is a live federated connection: the multiplexed session plus an http
// client for outbound calls over it. Caps holds the transports the remote
// peer advertised after the session was authenticated (zero value = relay
// only).
type Peer struct {
	ID      string
	Session *yamux.Session
	Client  *http.Client
	BaseURL string
	Caps    Capabilities
}

// Registry tracks live peer sessions by id. A peer present here is online.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]*Peer
}

func NewRegistry() *Registry { return &Registry{peers: make(map[string]*Peer)} }

func (r *Registry) put(p *Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.peers[p.ID]; old != nil {
		if old.Session != nil {
			old.Session.Close()
		}
	}
	r.peers[p.ID] = p
}

func (r *Registry) remove(id string, peer *Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if peer != nil && r.peers[id] != peer {
		return
	}
	delete(r.peers, id)
}

// Get returns the live peer for id, or nil if offline.
func (r *Registry) Get(id string) *Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[id]
}

// IDs returns the ids of all online peers.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.peers))
	for id := range r.peers {
		out = append(out, id)
	}
	return out
}

// dialHandshake is the member side: send "token peerID\n" on the raw conn, then
// read the single-byte ack the hub sends on success. Returns an error if the
// hub closes without acking (bad token or other rejection).
func dialHandshake(conn net.Conn, token, peerID string) error {
	if _, err := fmt.Fprintf(conn, "%s %s\n", token, peerID); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := conn.Read(ack[:]); err != nil {
		return fmt.Errorf("rejected: %w", err)
	}
	return nil
}

// acceptAndRegister performs the handshake, builds the yamux server session, and
// registers the peer. The caller serves over and tears down the session.
func acceptAndRegister(conn net.Conn, token string, reg *Registry) (*Peer, error) {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 {
		conn.Close()
		return nil, fmt.Errorf("malformed handshake")
	}
	gotToken, peerID := fields[0], fields[1]
	if subtle.ConstantTimeCompare([]byte(gotToken), []byte(token)) != 1 {
		conn.Close()
		return nil, fmt.Errorf("bad token")
	}
	// Send a single-byte ack before handing off to yamux so the member side
	// can detect rejection vs. acceptance without stealing yamux framing bytes.
	if _, err := conn.Write([]byte{1}); err != nil {
		conn.Close()
		return nil, err
	}
	sess, err := yamux.Server(&bufferedConn{Reader: br, Conn: conn}, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	p := &Peer{ID: peerID, Session: sess, Client: SessionClient(sess), BaseURL: "http://" + peerID}
	reg.put(p)
	return p, nil
}

// acceptPeer is the test-friendly form: register then block until the session
// dies. Production code uses acceptAndRegister + http.Serve.
func acceptPeer(conn net.Conn, token string, reg *Registry) error {
	p, err := acceptAndRegister(conn, token, reg)
	if err != nil {
		return err
	}
	defer reg.remove(p.ID, p)
	<-p.Session.CloseChan()
	return nil
}

// bufferedConn lets yamux read through a bufio.Reader that may already hold
// bytes buffered after the handshake line, while writes go straight to the conn.
type bufferedConn struct {
	*bufio.Reader
	net.Conn
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.Reader.Read(p) }
