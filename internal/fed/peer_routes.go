package fed

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// PeerRoutes is the innermost layer of the direct peer session handler (see
// PeerSessionHandler): this instance's federation routes plus
// peerVisibleAppRoutes of app. It deliberately does not
// mount app at "/" — see peerVisibleAppRoutes for why a catch-all here is a
// hole rather than a convenience.
func PeerRoutes(db *sql.DB, app http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fed/catalog", func(w http.ResponseWriter, r *http.Request) {
		rows, err := store.ExportCatalog(db)
		if err != nil {
			http.Error(w, "export catalog", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	})
	mountAppRoutes(mux, app)
	return mux
}

func PullPeerCatalog(db *sql.DB, client *http.Client, peerID, baseURL string) error {
	if client == nil {
		return nil
	}
	if baseURL == "" {
		baseURL = "http://" + peerID
	}
	resp, err := client.Get(baseURL + "/fed/catalog")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var rows []store.CatalogRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return err
	}
	return ApplyCatalog(db, peerID, rows)
}
