package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

var apiPathHomeDir = os.UserHomeDir

type librariesResponse struct {
	LocalLibraries []store.LocalLibrary        `json:"local_libraries"`
	Warnings       []store.LocalLibraryWarning `json:"warnings"`
	Federation     federationResponse          `json:"federation"`
	Scan           store.LibraryScanSettings   `json:"scan"`
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
	LocalLibraries []store.LocalLibrary      `json:"local_libraries"`
	Federation     store.FederationSettings  `json:"federation"`
	Scan           store.LibraryScanSettings `json:"scan"`
	SaveAndScan    bool                      `json:"save_and_scan"`
}

type libraryPathEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type libraryPathsResponse struct {
	Path        string             `json:"path"`
	Parent      string             `json:"parent,omitempty"`
	Directories []libraryPathEntry `json:"directories"`
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
	if err := store.SaveLibraryScanSettings(s.db, req.Scan); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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

func (s *Server) listLibraryPaths(w http.ResponseWriter, r *http.Request) {
	requestedValues, hasRequestedPath := r.URL.Query()["path"]
	requestedPath := ""
	if hasRequestedPath && len(requestedValues) > 0 {
		requestedPath = requestedValues[0]
	}
	if !hasRequestedPath {
		defaultPath, err := s.defaultLibraryPathStart()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		requestedPath = defaultPath
	}

	cleanedPath, status, err := cleanLibraryBrowserPath(requestedPath)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}

	info, err := os.Stat(cleanedPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("path is not readable: %v", err))
		return
	}
	if !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	entries, err := os.ReadDir(cleanedPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("path is not readable: %v", err))
		return
	}

	directories := make([]libraryPathEntry, 0, len(entries))
	for _, entry := range entries {
		if isHiddenLibraryPathEntry(entry.Name()) {
			continue
		}
		childPath := filepath.Clean(filepath.Join(cleanedPath, entry.Name()))
		if !isReadableDirectory(childPath) {
			continue
		}
		directories = append(directories, libraryPathEntry{Name: entry.Name(), Path: childPath})
	}
	sort.SliceStable(directories, func(i, j int) bool {
		leftName := strings.ToLower(directories[i].Name)
		rightName := strings.ToLower(directories[j].Name)
		if leftName == rightName {
			return directories[i].Name < directories[j].Name
		}
		return leftName < rightName
	})

	parent := filepath.Dir(cleanedPath)
	if parent == cleanedPath {
		parent = ""
	}
	writeJSON(w, http.StatusOK, libraryPathsResponse{Path: cleanedPath, Parent: parent, Directories: directories})
}

func isHiddenLibraryPathEntry(name string) bool {
	return strings.HasPrefix(name, ".") || name == "@eaDir"
}

func cleanLibraryBrowserPath(path string) (string, int, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return "", http.StatusBadRequest, fmt.Errorf("path cannot be blank")
	}
	if trimmedPath == "~" {
		home, err := apiPathHomeDir()
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		if strings.TrimSpace(home) == "" {
			return "", http.StatusInternalServerError, fmt.Errorf("home directory is blank")
		}
		return filepath.Clean(home), http.StatusOK, nil
	}
	if strings.HasPrefix(trimmedPath, "~/") || strings.HasPrefix(trimmedPath, `~\`) {
		home, err := apiPathHomeDir()
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
		if strings.TrimSpace(home) == "" {
			return "", http.StatusInternalServerError, fmt.Errorf("home directory is blank")
		}
		return filepath.Clean(filepath.Join(home, trimmedPath[2:])), http.StatusOK, nil
	}
	if strings.HasPrefix(trimmedPath, "~") {
		return "", http.StatusBadRequest, fmt.Errorf("unsupported home path %q", trimmedPath)
	}
	return filepath.Clean(trimmedPath), http.StatusOK, nil
}

func (s *Server) defaultLibraryPathStart() (string, error) {
	libraries, err := store.ListLocalLibraries(s.db)
	if err != nil {
		return "", err
	}
	for _, library := range libraries {
		cleanedPath := filepath.Clean(library.Path)
		if isReadableDirectory(cleanedPath) {
			return cleanedPath, nil
		}
	}

	home, err := apiPathHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Clean(home), nil
	}
	return string(filepath.Separator), nil
}

func isReadableDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}
	if _, err := os.ReadDir(path); err != nil {
		return false
	}
	return true
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
	scanSettings, err := store.LoadLibraryScanSettings(s.db)
	if err != nil {
		return librariesResponse{}, err
	}
	return librariesResponse{
		LocalLibraries: libs,
		Warnings:       store.LocalLibraryWarnings(libs),
		Scan:           scanSettings,
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
