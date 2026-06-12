CREATE TABLE IF NOT EXISTS artist (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS album (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    artist_id INTEGER NOT NULL REFERENCES artist(id),
    cover     TEXT NOT NULL DEFAULT '',
    UNIQUE(name, artist_id)
);
CREATE TABLE IF NOT EXISTS track (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT NOT NULL UNIQUE,
    mod_time   INTEGER NOT NULL,
    size       INTEGER NOT NULL,
    title      TEXT NOT NULL,
    artist_id  INTEGER NOT NULL REFERENCES artist(id),
    album_id   INTEGER NOT NULL REFERENCES album(id),
    track_no   INTEGER NOT NULL DEFAULT 0,
    genre      TEXT NOT NULL DEFAULT '',
    duration   INTEGER NOT NULL DEFAULT 0,
    play_count INTEGER NOT NULL DEFAULT 0,
    added_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_track_artist ON track(artist_id);
CREATE INDEX IF NOT EXISTS idx_track_album  ON track(album_id);

CREATE TABLE IF NOT EXISTS stream (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'private'
);
CREATE TABLE IF NOT EXISTS queue_item (
    stream_id  TEXT NOT NULL REFERENCES stream(id),
    track_id   INTEGER NOT NULL REFERENCES track(id),
    play_order INTEGER NOT NULL,
    added_by   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (stream_id, track_id)
);
CREATE TABLE IF NOT EXISTS history (
    stream_id TEXT NOT NULL,
    track_id  INTEGER NOT NULL,
    played_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_history_stream ON history(stream_id, played_at);
CREATE TABLE IF NOT EXISTS station (
    stream_id TEXT PRIMARY KEY REFERENCES stream(id),
    genre     TEXT NOT NULL,
    threshold INTEGER NOT NULL,
    batch     INTEGER NOT NULL
);
