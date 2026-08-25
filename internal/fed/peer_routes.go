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
//
// selfPeerID is this instance's own peer id, which /fed/catalog needs to decide
// whether the requesting peer shares a listening group with it. The audio route
// mounted alongside carries no such check — see the note in groups.go.
func PeerRoutes(db *sql.DB, selfPeerID string, app http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fed/catalog", func(w http.ResponseWriter, r *http.Request) {
		rows := []store.CatalogRow{}
		if catalogVisible(db, selfPeerID, SessionPeer(r)) {
			var err error
			rows, err = store.ExportCatalog(db)
			if err != nil {
				http.Error(w, "export catalog", http.StatusInternalServerError)
				return
			}
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
