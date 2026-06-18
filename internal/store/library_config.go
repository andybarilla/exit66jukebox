package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const keyLibrarySettingsInitialized = "library_settings_initialized"

var libraryPathHomeDir = os.UserHomeDir

type LocalLibrary struct {
	ID              int64  `json:"id"`
	Path            string `json:"path"`
	SourceLibraryID string `json:"source_library_id"`
	Enabled         bool   `json:"enabled"`
	Name            string `json:"name"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type FederationSettings struct {
	Enabled bool   `json:"enabled"`
	Role    string `json:"role"`
	HubAddr string `json:"hub_addr"`
	Listen  string `json:"listen"`
	Token   string `json:"token,omitempty"`
	PeerID  string `json:"peer_id"`
}

type LocalLibraryWarning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func LibrarySettingsInitialized(db *sql.DB) bool {
	return metaFlag(db, keyLibrarySettingsInitialized)
}

func SetLibrarySettingsInitialized(db *sql.DB) error {
	return setMetaFlag(db, keyLibrarySettingsInitialized, true)
}

func SeedLocalLibraries(db *sql.DB, roots []string) error {
	if LibrarySettingsInitialized(db) || len(roots) == 0 {
		return nil
	}
	libs := make([]LocalLibrary, 0, len(roots))
	for _, root := range roots {
		libs = append(libs, LocalLibrary{Path: root, Enabled: true})
	}
	return saveLocalLibraries(db, libs, true)
}

func SaveLocalLibraries(db *sql.DB, libs []LocalLibrary) error {
	return saveLocalLibraries(db, libs, true)
}

func EnsureLocalLibrary(db *sql.DB, path, name string) (int64, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." {
		path = ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = path
	}
	sourceLibraryID, err := newLocalSourceLibraryID()
	if err != nil {
		return 0, err
	}
	if _, err := db.Exec(
		`INSERT INTO local_library(path, source_library_id, enabled, name, created_at, updated_at)
		 VALUES(?, ?, 1, ?, strftime('%s','now'), strftime('%s','now'))
		 ON CONFLICT(path) DO UPDATE SET name = COALESCE(NULLIF(excluded.name, ''), local_library.name), updated_at = strftime('%s','now')`,
		path, sourceLibraryID, name,
	); err != nil {
		return 0, err
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM local_library WHERE path = ?`, path).Scan(&id)
	return id, err
}

func EnsureRemoteLibrary(db *sql.DB, peer, sourceLibraryID, name string) (int64, error) {
	peer = strings.TrimSpace(peer)
	sourceLibraryID = strings.TrimSpace(sourceLibraryID)
	if peer == "" {
		return 0, errors.New("remote library peer cannot be blank")
	}
	if sourceLibraryID == "" {
		sourceLibraryID = DefaultRemoteSourceLibraryID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = sourceLibraryID
	}
	if _, err := db.Exec(
		`INSERT INTO remote_library(source_peer, source_library_id, name, created_at, updated_at)
		 VALUES(?, ?, ?, strftime('%s','now'), strftime('%s','now'))
		 ON CONFLICT(source_peer, source_library_id) DO UPDATE SET name = excluded.name, updated_at = strftime('%s','now')`,
		peer, sourceLibraryID, name,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(
		`SELECT id FROM remote_library WHERE source_peer = ? AND source_library_id = ?`, peer, sourceLibraryID,
	).Scan(&id)
	return id, err
}

func SaveLibraryConfiguration(db *sql.DB, libs []LocalLibrary, settings FederationSettings) error {
	normalizedLibs, err := normalizeLocalLibraries(libs)
	if err != nil {
		return err
	}
	settings = normalizeFederationSettings(settings)
	if err := validateFederationSettings(settings); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveLocalLibrariesTx(tx, normalizedLibs, true); err != nil {
		return err
	}
	if err := saveFederationSettingsTx(tx, settings); err != nil {
		return err
	}
	return tx.Commit()
}

func saveLocalLibraries(db *sql.DB, libs []LocalLibrary, markInitialized bool) error {
	normalized, err := normalizeLocalLibraries(libs)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveLocalLibrariesTx(tx, normalized, markInitialized); err != nil {
		return err
	}
	return tx.Commit()
}

func saveLocalLibrariesTx(tx *sql.Tx, libs []LocalLibrary, markInitialized bool) error {
	keepIDs := make([]int64, 0, len(libs))
	for _, lib := range libs {
		sourceLibraryID, err := newLocalSourceLibraryID()
		if err != nil {
			return err
		}
		if lib.ID > 0 {
			if _, err := tx.Exec(
				`INSERT INTO local_library(id, path, source_library_id, enabled, name, created_at, updated_at)
				 VALUES(?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))
				 ON CONFLICT(id) DO UPDATE SET
				   path=excluded.path, enabled=excluded.enabled, name=excluded.name,
				   updated_at=strftime('%s','now')`,
				lib.ID, lib.Path, sourceLibraryID, boolToInt(lib.Enabled), lib.Name,
			); err != nil {
				return err
			}
			keepIDs = append(keepIDs, lib.ID)
			continue
		}
		var existingID int64
		if err := tx.QueryRow(`SELECT id FROM local_library WHERE path = ?`, lib.Path).Scan(&existingID); err == nil {
			if _, err := tx.Exec(
				`UPDATE local_library SET enabled = ?, name = ?, updated_at = strftime('%s','now') WHERE id = ?`,
				boolToInt(lib.Enabled), lib.Name, existingID,
			); err != nil {
				return err
			}
			keepIDs = append(keepIDs, existingID)
			continue
		} else if err != sql.ErrNoRows {
			return err
		}
		res, err := tx.Exec(
			`INSERT INTO local_library(path, source_library_id, enabled, name, created_at, updated_at)
			 VALUES(?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))`,
			lib.Path, sourceLibraryID, boolToInt(lib.Enabled), lib.Name,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		keepIDs = append(keepIDs, id)
	}
	if len(keepIDs) == 0 {
		if _, err := tx.Exec(`DELETE FROM local_library`); err != nil {
			return err
		}
	} else {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(keepIDs)), ",")
		args := make([]any, 0, len(keepIDs))
		for _, id := range keepIDs {
			args = append(args, id)
		}
		if _, err := tx.Exec(`DELETE FROM local_library WHERE id NOT IN (`+placeholders+`)`, args...); err != nil {
			return err
		}
	}
	if markInitialized {
		if _, err := tx.Exec(
			`INSERT INTO meta(key, value) VALUES(?, 1)
			 ON CONFLICT(key) DO UPDATE SET value = 1`, keyLibrarySettingsInitialized,
		); err != nil {
			return err
		}
	}
	return nil
}

func normalizeLocalLibraries(libs []LocalLibrary) ([]LocalLibrary, error) {
	seen := map[string]bool{}
	normalized := make([]LocalLibrary, 0, len(libs))
	for _, lib := range libs {
		path, err := expandLocalLibraryPath(lib.Path)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, fmt.Errorf("duplicate library path: %s", path)
		}
		seen[path] = true
		lib.Path = path
		lib.Name = strings.TrimSpace(lib.Name)
		normalized = append(normalized, lib)
	}
	return normalized, nil
}

func expandLocalLibraryPath(path string) (string, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return "", errors.New("library path cannot be blank")
	}
	if trimmedPath == "~" {
		return libraryPathHome()
	}
	if strings.HasPrefix(trimmedPath, "~/") {
		home, err := libraryPathHome()
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(home, trimmedPath[2:])), nil
	}
	if strings.HasPrefix(trimmedPath, "~") {
		return "", fmt.Errorf("unsupported library path %q: only ~ and ~/ paths are supported", trimmedPath)
	}
	return filepath.Clean(trimmedPath), nil
}

func libraryPathHome() (string, error) {
	home, err := libraryPathHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for library path: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("resolve home directory for library path: home directory is blank")
	}
	return filepath.Clean(home), nil
}

func ListLocalLibraries(db *sql.DB) ([]LocalLibrary, error) {
	rows, err := db.Query(
		`SELECT id, path, source_library_id, enabled, name, created_at, updated_at FROM local_library ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var libs []LocalLibrary
	for rows.Next() {
		var lib LocalLibrary
		var enabled int
		if err := rows.Scan(&lib.ID, &lib.Path, &lib.SourceLibraryID, &enabled, &lib.Name, &lib.CreatedAt, &lib.UpdatedAt); err != nil {
			return nil, err
		}
		lib.Enabled = enabled != 0
		libs = append(libs, lib)
	}
	return libs, rows.Err()
}

func newLocalSourceLibraryID() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate local library identity: %w", err)
	}
	return "local-" + hex.EncodeToString(randomBytes), nil
}

func EnabledLocalLibraryRoots(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM local_library WHERE enabled != 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

func LocalLibraryWarnings(libs []LocalLibrary) []LocalLibraryWarning {
	warnings := []LocalLibraryWarning{}
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		info, err := os.Stat(lib.Path)
		if err != nil {
			warnings = append(warnings, LocalLibraryWarning{Path: lib.Path, Message: "path is missing or unreadable"})
			continue
		}
		if !info.IsDir() {
			warnings = append(warnings, LocalLibraryWarning{Path: lib.Path, Message: "path is not a directory"})
		}
	}
	return warnings
}

func SaveFederationSettings(db *sql.DB, settings FederationSettings) error {
	settings = normalizeFederationSettings(settings)
	if err := validateFederationSettings(settings); err != nil {
		return err
	}
	_, err := db.Exec(federationSettingsUpsertSQL,
		boolToInt(settings.Enabled), settings.Role, settings.HubAddr, settings.Listen, settings.Token, settings.PeerID,
	)
	return err
}

const federationSettingsUpsertSQL = `INSERT INTO federation_settings(id, enabled, role, hub_addr, listen, token, peer_id, created_at, updated_at)
	 VALUES(1, ?, ?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))
	 ON CONFLICT(id) DO UPDATE SET
	   enabled=excluded.enabled, role=excluded.role, hub_addr=excluded.hub_addr,
	   listen=excluded.listen, token=excluded.token, peer_id=excluded.peer_id,
	   updated_at=strftime('%s','now')`

func saveFederationSettingsTx(tx *sql.Tx, settings FederationSettings) error {
	_, err := tx.Exec(federationSettingsUpsertSQL,
		boolToInt(settings.Enabled), settings.Role, settings.HubAddr, settings.Listen, settings.Token, settings.PeerID,
	)
	return err
}

func LoadFederationSettings(db *sql.DB) (FederationSettings, bool, error) {
	var settings FederationSettings
	var enabled int
	err := db.QueryRow(
		`SELECT enabled, role, hub_addr, listen, token, peer_id FROM federation_settings WHERE id = 1`,
	).Scan(&enabled, &settings.Role, &settings.HubAddr, &settings.Listen, &settings.Token, &settings.PeerID)
	if err == sql.ErrNoRows {
		return FederationSettings{}, false, nil
	}
	if err != nil {
		return FederationSettings{}, false, err
	}
	settings.Enabled = enabled != 0
	return settings, true, nil
}

func normalizeFederationSettings(settings FederationSettings) FederationSettings {
	settings.Role = strings.TrimSpace(settings.Role)
	settings.HubAddr = strings.TrimSpace(settings.HubAddr)
	settings.Listen = strings.TrimSpace(settings.Listen)
	settings.Token = strings.TrimSpace(settings.Token)
	settings.PeerID = strings.TrimSpace(settings.PeerID)
	if !settings.Enabled {
		settings.Role = ""
	}
	return settings
}

func validateFederationSettings(settings FederationSettings) error {
	if !settings.Enabled {
		return nil
	}
	if settings.Role != "hub" && settings.Role != "member" && settings.Role != "peer" {
		return errors.New("federation role must be hub, member, or peer")
	}
	if settings.Token == "" {
		return errors.New("federation token is required")
	}
	if settings.PeerID == "" {
		return errors.New("federation peer id is required")
	}
	if (settings.Role == "hub" || settings.Role == "peer") && settings.Listen == "" {
		return fmt.Errorf("federation listen address is required for %s role", settings.Role)
	}
	if settings.Role == "member" && settings.HubAddr == "" {
		return errors.New("federation hub address is required for member role")
	}
	return nil
}

func FederationSettingsEqual(a, b FederationSettings) bool {
	return comparableFederationSettings(a) == comparableFederationSettings(b)
}

func comparableFederationSettings(settings FederationSettings) FederationSettings {
	settings = normalizeFederationSettings(settings)
	if settings.Enabled {
		return settings
	}
	settings.HubAddr = ""
	settings.Listen = ""
	settings.Token = ""
	settings.PeerID = ""
	return settings
}
