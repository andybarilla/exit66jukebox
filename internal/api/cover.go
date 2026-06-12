package api

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dhowden/tag"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) trackCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	serveCover(w, s, id)
}

func (s *Server) albumCover(w http.ResponseWriter, r *http.Request) {
	albumID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	trackID, ok := store.FirstTrackIDOfAlbum(s.db, albumID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	serveCover(w, s, trackID)
}

// serveCover writes a track's cover image. It prefers the file's embedded art;
// when there is none (or the file can't be read), it falls back to the
// album-keyed cover cached by the enrichment pass. 404 if neither exists.
func serveCover(w http.ResponseWriter, s *Server, trackID int64) {
	if ct, data, ok := embeddedCover(s.db, trackID); ok {
		writeImage(w, ct, data)
		return
	}
	if path, ok := store.AlbumCoverByTrack(s.db, trackID); ok {
		if serveCoverFile(w, path) {
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// embeddedCover reads a track's embedded picture, returning ok=false on any
// failure (missing file, unparseable tags, or no picture).
func embeddedCover(db *sql.DB, trackID int64) (contentType string, data []byte, ok bool) {
	_, path, found := store.GetTrack(db, trackID)
	if !found {
		return "", nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, false
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", nil, false
	}
	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return "", nil, false
	}
	ct := pic.MIMEType
	if !strings.HasPrefix(ct, "image/") {
		ct = http.DetectContentType(pic.Data) // tag MIME missing/garbage; sniff the bytes
	}
	return ct, pic.Data, true
}

// serveCoverFile streams a cached cover file, sniffing its content type.
// Returns false if the file can't be opened (caller then 404s).
func serveCoverFile(w http.ResponseWriter, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false
	}
	w.Header().Set("Content-Type", http.DetectContentType(head[:n]))
	w.Header().Set("Cache-Control", "max-age=86400")
	io.Copy(w, f)
	return true
}

func writeImage(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(data)
}
