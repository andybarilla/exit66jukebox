package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) listFederationPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := store.ListFederationPeers(s.db, r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}

func (s *Server) addFederationPeer(w http.ResponseWriter, r *http.Request) {
	var peer store.FederationPeer
	if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	peer.Status = store.PeerStatusAccepted
	peer.Manual = true
	peer.TokenAuthenticated = true
	if err := store.SaveFederationPeer(s.db, peer); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationPeers(w, r)
}

func (s *Server) approveFederationPeer(w http.ResponseWriter, r *http.Request) {
	peerID := strings.TrimSpace(r.PathValue("peerID"))
	if peerID == "" {
		writeErr(w, http.StatusBadRequest, "peer id is required")
		return
	}
	if err := store.ApproveFederationPeer(s.db, peerID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationPeers(w, r)
}
