package fed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// ApplyCatalog reconciles the cached rows for peer to the given catalog: upsert
// each incoming row (which preserves its local id via ON CONFLICT), then delete
// only the peer's rows that are no longer present. Reconciling in place keeps a
// track's local id STABLE across syncs — a blanket delete+reinsert would mint a
// fresh autoincrement id every sync cycle, 404ing ids a client already holds
// (browse→play) and orphaning queue rows.
//
// An EMPTY catalog deletes everything cached for peer. Listening groups rely on
// that: a peer denied by group membership is answered with an empty catalog
// rather than an error, which is what makes revoking membership remove the rows
// it already holds (#88).
func ApplyCatalog(db *sql.DB, peer string, rows []store.CatalogRow) error {
	if len(rows) == 0 {
		return store.DeleteRemoteTracks(db, peer)
	}
	keepByLibrary := map[string][]int64{}
	for _, c := range rows {
		sourceLibraryID := c.SourceLibraryID
		if sourceLibraryID == "" {
			sourceLibraryID = store.DefaultRemoteSourceLibraryID
		}
		if _, err := store.UpsertRemoteTrack(db, store.RemoteTrack{
			SourcePeer: peer, SourceLibraryID: sourceLibraryID, SourceLibraryName: c.SourceLibraryName, RemoteID: c.RemoteID,
			Title: c.Title, ArtistName: c.ArtistName, AlbumArtist: c.AlbumArtist,
			AlbumName: c.AlbumName, TrackNo: c.TrackNo, Genre: c.Genre,
			Duration: c.Duration, Links: c.Links,
		}); err != nil {
			return err
		}
		keepByLibrary[sourceLibraryID] = append(keepByLibrary[sourceLibraryID], c.RemoteID)
	}
	sourceLibraryIDs, err := store.RemoteSourceLibraryIDs(db, peer)
	if err != nil {
		return err
	}
	for _, sourceLibraryID := range sourceLibraryIDs {
		keep := keepByLibrary[sourceLibraryID]
		if err := store.DeleteRemoteLibraryTracksExcept(db, peer, sourceLibraryID, keep); err != nil {
			return err
		}
	}
	return nil
}

// PullAndApply (member side) fetches the merged catalog from the hub, applies
// each peer's rows into the local DB, and returns the hub-reported list of
// online peers so the caller can update liveness.
func PullAndApply(db *sql.DB, hubClient *http.Client, peerID string) ([]string, error) {
	resp, err := hubClient.Get("http://@hub/fed/catalog/" + peerID + "/merged")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var merged MergedCatalog
	if err := json.NewDecoder(resp.Body).Decode(&merged); err != nil {
		return nil, err
	}
	for peer, rows := range merged.Catalogs {
		if err := ApplyCatalog(db, peer, rows); err != nil {
			return nil, err
		}
	}
	return merged.Online, nil
}

// PushCatalog (member side) exports the local catalog and POSTs it to the hub.
func PushCatalog(db *sql.DB, hubClient *http.Client, peerID string) error {
	rows, err := store.ExportCatalog(db)
	if err != nil {
		return err
	}
	body, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "http://@hub/fed/catalog/"+peerID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hubClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
