package fed

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/hashicorp/yamux"
)

// Manager runs federation for one instance. Role "hub" listens for members;
// role "member" dials a hub and keeps the connection alive. MemberHandler is
// the http.Handler served back to the hub over the session (the member's audio
// endpoints); HubHandler is served to members (the relay endpoints).
type Manager struct {
	Role          string
	Token         string
	PeerID        string
	HubAddr       string       // member only: hub to dial
	HubListen     string       // hub only: local listen addr
	MemberHandler http.Handler // served over session, member side
	HubHandler    http.Handler // served over session, hub side
	PeerHandler   http.Handler // served over direct peer sessions
	Registry      *Registry
	Relay         *Relay  // hub role: the single relay instance shared with HubHandler
	DB            *sql.DB // member: local DB the sync loop pushes/pulls against
	PeerAddrs     map[string]string
	Caps          Capabilities     // this instance's advertised transports
	Signaler      *Signaler        // relays WebRTC signaling between authenticated peers
	WebRTC        *WebRTCTransport // direct NAT-traversing audio transport (peer role)

	mu         sync.Mutex      // guards online
	online     []string        // member: peers the hub last reported online
	hubSession *yamux.Session  // member side: the live session to the hub
	ctx        context.Context // optional lifetime context for background loops
}

// Start launches the role's networking in background goroutines.
func (m *Manager) Start() {
	if m.Registry == nil {
		m.Registry = NewRegistry()
	}
	if m.Signaler == nil {
		m.Signaler = NewSignaler()
	}
	switch m.Role {
	case "hub":
		go m.runHub()
	case "member":
		go m.runMember()
	case "peer":
		go m.runPeer()
	}
	// Start the WebRTC transport's answerer loop (also serves the offerer's own
	// reply mailbox). It serves inbound data channels against the app handler so
	// remote peers can stream this instance's audio directly. PeerHandler already
	// routes /api/tracks/{id}/audio to the local app handler.
	if m.WebRTC != nil {
		ctx := m.rootCtx()
		appHandler := m.PeerHandler
		if appHandler == nil {
			appHandler = m.MemberHandler
		}
		m.WebRTC.Listen(ctx, func(conn *dataChannelConn) {
			go func() {
				// Closing when the peer is done is what releases the answering
				// PeerConnection and its signaling slot; a detached data channel
				// reports the far end going away as an EOF here and nowhere else.
				defer conn.Close()
				if err := ServeAudioOverConn(conn, appHandler, ""); err != nil {
					log.Printf("fed webrtc serve %s: %v", conn.peerID, err)
				}
			}()
		})
	}
}

// SetContext supplies a lifetime context for background loops (e.g. the WebRTC
// answerer). Call before Start.
func (m *Manager) SetContext(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
}

// rootCtx returns a background context for the manager's lifetime when no
// cancellation context was supplied.
func (m *Manager) rootCtx() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Manager) runHub() {
	ln, err := net.Listen("tcp", m.HubListen)
	if err != nil {
		log.Printf("fed hub listen %s: %v", m.HubListen, err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go m.serveHubConn(conn)
	}
}

func (m *Manager) runPeer() {
	if m.HubAddr != "" {
		go m.runMember()
	}
	go m.runPeerListener()
	go m.runPeerDialer()
}

func (m *Manager) runPeerListener() {
	ln, err := net.Listen("tcp", m.HubListen)
	if err != nil {
		log.Printf("fed peer listen %s: %v", m.HubListen, err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go m.servePeerConn(conn)
	}
}

func (m *Manager) servePeerConn(conn net.Conn) {
	p, err := acceptAndRegister(conn, m.Token, m.Registry)
	if err != nil {
		return
	}
	defer m.Registry.remove(p.ID, p)
	if !m.acceptsPeer(p.ID) {
		if m.DB != nil {
			_ = store.SaveFederationPeer(m.DB, store.FederationPeer{PeerID: p.ID, Address: conn.RemoteAddr().String(), Status: store.PeerStatusPending, TokenAuthenticated: true})
		}
		p.Session.Close()
		return
	}
	if m.DB != nil {
		_ = store.MarkFederationPeerAuthenticated(m.DB, p.ID)
	}
	m.learnCaps(p)
	go m.startDirectSyncLoop(p, p.Session.CloseChan())
	if m.PeerHandler != nil {
		_ = http.Serve(p.Session, m.PeerHandler)
		return
	}
	<-p.Session.CloseChan()
}

func (m *Manager) runPeerDialer() {
	for {
		m.runPeerDialerOnce()
		time.Sleep(30 * time.Second)
	}
}

func (m *Manager) runPeerDialerOnce() {
	for peerID, addr := range m.peerAddrs() {
		if m.Registry.Get(peerID) != nil {
			continue
		}
		go m.dialPeer(peerID, addr)
	}
}

func (m *Manager) dialPeer(peerID, addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return
	}
	if err := dialHandshake(conn, m.Token, m.PeerID); err != nil {
		conn.Close()
		return
	}
	sess, err := yamux.Client(conn, nil)
	if err != nil {
		conn.Close()
		return
	}
	p := &Peer{ID: peerID, Session: sess, Client: SessionClient(sess), BaseURL: "http://" + peerID}
	m.Registry.put(p)
	m.learnCaps(p)
	go m.startDirectSyncLoop(p, sess.CloseChan())
	if m.PeerHandler != nil {
		_ = http.Serve(sess, m.PeerHandler)
	} else {
		<-sess.CloseChan()
	}
	m.Registry.remove(peerID, p)
	sess.Close()
}

func (m *Manager) peerAddrs() map[string]string {
	if m.DB == nil {
		return m.PeerAddrs
	}
	addrs, err := store.AcceptedFederationPeerAddresses(m.DB)
	if err != nil {
		return m.PeerAddrs
	}
	return addrs
}

func (m *Manager) acceptsPeer(peerID string) bool {
	_, ok := m.peerAddrs()[peerID]
	return ok
}

func (m *Manager) startDirectSyncLoop(peer *Peer, done <-chan struct{}) {
	if m.DB == nil {
		return
	}
	m.directSyncOnce(peer)
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			m.directSyncOnce(peer)
		}
	}
}

func (m *Manager) directSyncOnce(peer *Peer) {
	if peer == nil {
		return
	}
	_ = PullPeerCatalog(m.DB, peer.Client, peer.ID, peer.BaseURL)
}

// learnCaps fetches the remote peer's capabilities over its authenticated
// session and records them on the Peer so the resolver can pick a transport.
// Best-effort: a zero value (relay only) is left in place on any error.
func (m *Manager) learnCaps(peer *Peer) {
	if peer == nil {
		return
	}
	peer.Caps = fetchCaps(peer.Client, peer.BaseURL)
}

// serveHubConn performs the handshake, registers the peer, and serves HubHandler
// over the session until it closes, tagged with the peer id that session belongs
// to.
func (m *Manager) serveHubConn(conn net.Conn) {
	p, err := acceptAndRegister(conn, m.Token, m.Registry)
	if err != nil {
		return
	}
	defer m.Registry.remove(p.ID, p)
	if m.HubHandler != nil {
		// One HubHandler serves every member session, so the id of the peer on
		// the far end can only come from here. The signaling relay stamps it into
		// the messages it forwards; without it those are refused (#158).
		_ = http.Serve(p.Session, WithSessionPeer(p.ID, m.HubHandler))
	} else {
		<-p.Session.CloseChan()
	}
}

func (m *Manager) runMember() {
	backoff := time.Second
	for {
		conn, err := net.Dial("tcp", m.HubAddr)
		if err != nil {
			time.Sleep(backoff)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		if err := dialHandshake(conn, m.Token, m.PeerID); err != nil {
			conn.Close()
			time.Sleep(backoff)
			continue
		}
		sess, err := yamux.Client(conn, nil)
		if err != nil {
			conn.Close()
			time.Sleep(backoff)
			continue
		}
		m.hubSession = sess
		// Register the hub as a pseudo-peer so the member can call it by client.
		p := &Peer{ID: "@hub", Session: sess, Client: SessionClient(sess), BaseURL: "http://@hub"}
		m.Registry.put(p)
		go m.startSyncLoop(sess.CloseChan())
		backoff = time.Second
		if m.MemberHandler != nil {
			_ = http.Serve(sess, m.MemberHandler) // blocks until session closes
		} else {
			<-sess.CloseChan()
		}
		m.Registry.remove("@hub", p)
		sess.Close()
	}
}

// HubClient returns the http client to the hub (member side), or nil if not
// connected.
func (m *Manager) HubClient() *http.Client {
	if p := m.Registry.Get("@hub"); p != nil {
		return p.Client
	}
	return nil
}

// startSyncLoop (member) pushes the local catalog and pulls the merged catalog
// on connect, then repeats on a ticker (push covers newly-scanned local tracks;
// pull refreshes remote rows + liveness). Runs until the session closes.
func (m *Manager) startSyncLoop(done <-chan struct{}) {
	if m.DB == nil {
		return
	}
	m.syncOnce()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			m.syncOnce()
		}
	}
}

func (m *Manager) syncOnce() {
	hc := m.HubClient()
	if hc == nil {
		return
	}
	_ = PushCatalog(m.DB, hc, m.PeerID)
	if online, err := PullAndApply(m.DB, hc, m.PeerID); err == nil {
		m.setOnline(online)
	}
}

func (m *Manager) setOnline(ids []string) {
	m.mu.Lock()
	m.online = ids
	m.mu.Unlock()
}

// OnlinePeers returns the ids of peers currently online. The hub knows this
// directly from its registry; a member uses the list the hub last reported,
// since a member's own registry only contains the hub.
func (m *Manager) OnlinePeers() []string {
	if m.Role == "hub" {
		return m.Registry.IDs()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.online...)
}
