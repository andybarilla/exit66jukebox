package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const keyLibrarySettingsInitialized = "library_settings_initialized"

type LocalLibrary struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Enabled   bool   `json:"enabled"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
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
	if _, err := tx.Exec(`DELETE FROM local_library`); err != nil {
		return err
	}
	for _, lib := range libs {
		if _, err := tx.Exec(
			`INSERT INTO local_library(path, enabled, name, created_at, updated_at)
			 VALUES(?, ?, ?, strftime('%s','now'), strftime('%s','now'))`,
			lib.Path, boolToInt(lib.Enabled), lib.Name,
		); err != nil {
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
		path := strings.TrimSpace(lib.Path)
		if path == "" {
			return nil, errors.New("library path cannot be blank")
		}
		path = filepath.Clean(path)
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

func ListLocalLibraries(db *sql.DB) ([]LocalLibrary, error) {
	rows, err := db.Query(
		`SELECT id, path, enabled, name, created_at, updated_at FROM local_library ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var libs []LocalLibrary
	for rows.Next() {
		var lib LocalLibrary
		var enabled int
		if err := rows.Scan(&lib.ID, &lib.Path, &enabled, &lib.Name, &lib.CreatedAt, &lib.UpdatedAt); err != nil {
			return nil, err
		}
		lib.Enabled = enabled != 0
		libs = append(libs, lib)
	}
	return libs, rows.Err()
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
	if settings.Role != "hub" && settings.Role != "member" {
		return errors.New("federation role must be hub or member")
	}
	if settings.Token == "" {
		return errors.New("federation token is required")
	}
	if settings.PeerID == "" {
		return errors.New("federation peer id is required")
	}
	if settings.Role == "hub" && settings.Listen == "" {
		return errors.New("federation listen address is required for hub role")
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
