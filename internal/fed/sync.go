package fed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// ApplyCatalog replaces all cached rows for peer with the given catalog rows:
// delete the peer's existing rows, then upsert the new set.
func ApplyCatalog(db *sql.DB, peer string, rows []store.CatalogRow) error {
	if err := store.DeleteRemoteTracks(db, peer); err != nil {
		return err
	}
	for _, c := range rows {
		if _, err := store.UpsertRemoteTrack(db, store.RemoteTrack{
			SourcePeer: peer, RemoteID: c.RemoteID,
			Title: c.Title, ArtistName: c.ArtistName, AlbumArtist: c.AlbumArtist,
			AlbumName: c.AlbumName, TrackNo: c.TrackNo, Genre: c.Genre,
			Duration: c.Duration, Links: c.Links,
		}); err != nil {
			return err
		}
	}
	return nil
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
