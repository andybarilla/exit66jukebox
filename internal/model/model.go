package model

// Artist is a distinct performer name.
type Artist struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Mbid string `json:"-"`
}

// Album belongs to one artist and may carry a cover image path.
type Album struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ArtistID int64  `json:"artist_id"`
	Cover    string `json:"-"`
	Mbid     string `json:"-"`
}

// Track is one audio file plus its indexed tags.
type Track struct {
	ID        int64  `json:"id"`
	Path      string `json:"-"`
	ModTime   int64  `json:"-"`
	Size      int64  `json:"-"`
	Title     string `json:"title"`
	ArtistID  int64  `json:"artist_id"`
	AlbumID   int64  `json:"album_id"`
	TrackNo   int    `json:"track_no"`
	Genre     string `json:"genre"`
	Duration  int    `json:"duration"`
	PlayCount int    `json:"play_count"`
	Mbid      string `json:"-"`
}

// Stream owns a queue + fairness config. Kind is "private" or "shared".
type Stream struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
