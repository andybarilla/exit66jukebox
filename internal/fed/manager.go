package fed

import (
	"log"
	"net"
	"net/http"
	"time"

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
	Registry      *Registry
	Relay         *Relay // hub role: the single relay instance shared with HubHandler

	hubSession *yamux.Session // member side: the live session to the hub
}

// Start launches the role's networking in background goroutines.
func (m *Manager) Start() {
	if m.Registry == nil {
		m.Registry = NewRegistry()
	}
	switch m.Role {
	case "hub":
		go m.runHub()
	case "member":
		go m.runMember()
	}
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

// serveHubConn performs the handshake, registers the peer, and serves HubHandler
// over the session until it closes.
func (m *Manager) serveHubConn(conn net.Conn) {
	p, err := acceptAndRegister(conn, m.Token, m.Registry)
	if err != nil {
		return
	}
	defer m.Registry.remove(p.ID)
	if m.HubHandler != nil {
		_ = http.Serve(p.Session, m.HubHandler)
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
		m.Registry.put(&Peer{ID: "@hub", Session: sess, Client: SessionClient(sess)})
		backoff = time.Second
		if m.MemberHandler != nil {
			_ = http.Serve(sess, m.MemberHandler) // blocks until session closes
		} else {
			<-sess.CloseChan()
		}
		m.Registry.remove("@hub")
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
