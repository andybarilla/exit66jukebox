package api

import (
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

type librariesResponse struct {
	LocalLibraries []store.LocalLibrary        `json:"local_libraries"`
	Warnings       []store.LocalLibraryWarning `json:"warnings"`
	Federation     federationResponse          `json:"federation"`
}

type federationResponse struct {
	Enabled         bool   `json:"enabled"`
	Role            string `json:"role"`
	HubAddr         string `json:"hub_addr"`
	Listen          string `json:"listen"`
	PeerID          string `json:"peer_id"`
	TokenConfigured bool   `json:"token_configured"`
	RestartRequired bool   `json:"restart_required"`
}

type saveLibrariesRequest struct {
	LocalLibraries []store.LocalLibrary     `json:"local_libraries"`
	Federation     store.FederationSettings `json:"federation"`
	SaveAndScan    bool                     `json:"save_and_scan"`
}

func (s *Server) getAdminLibraries(w http.ResponseWriter, r *http.Request) {
	resp, err := s.librarySettingsResponse()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) setAdminLibraries(w http.ResponseWriter, r *http.Request) {
	var req saveLibrariesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	settings := s.federationSettingsForSave(req.Federation)
	if err := store.SaveLibraryConfiguration(s.db, req.LocalLibraries, settings); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.SaveAndScan {
		if err := s.startLibraryScan(); err != nil {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
	}
	resp, err := s.librarySettingsResponse()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) federationSettingsForSave(in store.FederationSettings) store.FederationSettings {
	if in.Token != "" {
		return in
	}
	previous, ok, err := store.LoadFederationSettings(s.db)
	if err != nil || !ok {
		return in
	}
	in.Token = previous.Token
	return in
}

func (s *Server) librarySettingsResponse() (librariesResponse, error) {
	libs, err := store.ListLocalLibraries(s.db)
	if err != nil {
		return librariesResponse{}, err
	}
	fedSettings, _, err := store.LoadFederationSettings(s.db)
	if err != nil {
		return librariesResponse{}, err
	}
	return librariesResponse{
		LocalLibraries: libs,
		Warnings:       store.LocalLibraryWarnings(libs),
		Federation: federationResponse{
			Enabled:         fedSettings.Enabled,
			Role:            fedSettings.Role,
			HubAddr:         fedSettings.HubAddr,
			Listen:          fedSettings.Listen,
			PeerID:          fedSettings.PeerID,
			TokenConfigured: fedSettings.Token != "",
			RestartRequired: !store.FederationSettingsEqual(fedSettings, s.activeFed),
		},
	}, nil
}

func (s *Server) startLibraryScan() error {
	roots, err := store.EnabledLocalLibraryRoots(s.db)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return errNoEnabledLibraries{}
	}
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if s.scan != nil && s.scan.Snapshot().Running {
		return errScanAlreadyRunning{}
	}
	if s.scan == nil {
		s.scan = &scan.Progress{}
	}
	progress := s.scan
	progress.SetRunning(true)
	workers := s.scanWorkers
	go func() {
		defer progress.SetRunning(false)
		_, _ = scan.Scan(s.db, roots, workers, progress)
	}()
	return nil
}

type errNoEnabledLibraries struct{}

func (errNoEnabledLibraries) Error() string { return "no enabled libraries to scan" }

type errScanAlreadyRunning struct{}

func (errScanAlreadyRunning) Error() string { return "scan already running" }
