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

// EnrichedTrack is a Track plus the self-describing display fields the client
// used to derive from the whole in-memory library: the crate-wall slot code
// (album letter + track number), its tone, and the album/artist names.
type EnrichedTrack struct {
	Track
	Code       string `json:"code"`
	Tone       string `json:"tone"`
	AlbumName  string `json:"album_name"`
	ArtistName string `json:"artist_name"`
}

// EnrichedAlbum is an Album plus its globally-ranked crate-wall letter/tone, the
// artist name, and its track count.
type EnrichedAlbum struct {
	Album
	Letter     string `json:"letter"`
	Tone       string `json:"tone"`
	ArtistName string `json:"artist_name"`
	TrackCount int    `json:"track_count"`
}

// EnrichedArtist is an Artist plus its album and track counts.
type EnrichedArtist struct {
	Artist
	AlbumCount int `json:"album_count"`
	TrackCount int `json:"track_count"`
}
