package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Listening groups scope which peers discover each other's catalogs. They are
// not a playback boundary — a peer that already knows a track id can still
// fetch its audio whatever its membership (#88).

func (s *Server) listFederationGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := store.ListFederationGroups(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (s *Server) createFederationGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := store.CreateFederationGroup(s.db, req.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationGroups(w, r)
}

func (s *Server) deleteFederationGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := federationGroupID(w, r)
	if !ok {
		return
	}
	if err := store.DeleteFederationGroup(s.db, id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationGroups(w, r)
}

func (s *Server) addFederationGroupMember(w http.ResponseWriter, r *http.Request) {
	id, ok := federationGroupID(w, r)
	if !ok {
		return
	}
	var req struct {
		PeerID string `json:"peer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := store.AddFederationGroupMember(s.db, id, req.PeerID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationGroups(w, r)
}

func (s *Server) removeFederationGroupMember(w http.ResponseWriter, r *http.Request) {
	id, ok := federationGroupID(w, r)
	if !ok {
		return
	}
	peerID := strings.TrimSpace(r.PathValue("peerID"))
	if peerID == "" {
		writeErr(w, http.StatusBadRequest, "peer id is required")
		return
	}
	if err := store.RemoveFederationGroupMember(s.db, id, peerID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.listFederationGroups(w, r)
}

func federationGroupID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return 0, false
	}
	return id, true
}
