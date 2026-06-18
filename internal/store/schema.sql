CREATE TABLE IF NOT EXISTS artist (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL UNIQUE,
    sort_key TEXT NOT NULL DEFAULT '',
    mbid     TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS album (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    artist_id INTEGER NOT NULL REFERENCES artist(id),
    sort_key  TEXT NOT NULL DEFAULT '',
    cover     TEXT NOT NULL DEFAULT '',
    mbid      TEXT NOT NULL DEFAULT '',
    UNIQUE(name, artist_id)
);
CREATE INDEX IF NOT EXISTS idx_album_sortkey  ON album(sort_key, id);
CREATE INDEX IF NOT EXISTS idx_artist_sortkey ON artist(sort_key, id);
CREATE TABLE IF NOT EXISTS track (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT NOT NULL,
    mod_time   INTEGER NOT NULL,
    size       INTEGER NOT NULL,
    title      TEXT NOT NULL,
    artist_id  INTEGER NOT NULL REFERENCES artist(id),
    album_id   INTEGER NOT NULL REFERENCES album(id),
    track_no   INTEGER NOT NULL DEFAULT 0,
    genre      TEXT NOT NULL DEFAULT '',
    duration   INTEGER NOT NULL DEFAULT 0,
    play_count INTEGER NOT NULL DEFAULT 0,
    added_at   INTEGER NOT NULL DEFAULT 0,
    mbid        TEXT NOT NULL DEFAULT '',
    links       TEXT NOT NULL DEFAULT '',
    source_peer       TEXT NOT NULL DEFAULT '',
    source_library_id TEXT NOT NULL DEFAULT '',
    remote_id         INTEGER NOT NULL DEFAULT 0,
    library_id        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_track_artist ON track(artist_id);
CREATE INDEX IF NOT EXISTS idx_track_album  ON track(album_id);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS local_library (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT NOT NULL UNIQUE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    name       TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS remote_library (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    source_peer       TEXT NOT NULL,
    source_library_id TEXT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    UNIQUE(source_peer, source_library_id)
);
CREATE TABLE IF NOT EXISTS federation_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    enabled    INTEGER NOT NULL DEFAULT 0,
    role       TEXT NOT NULL DEFAULT '',
    hub_addr   TEXT NOT NULL DEFAULT '',
    listen     TEXT NOT NULL DEFAULT '',
    token      TEXT NOT NULL DEFAULT '',
    peer_id    TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS federation_peer (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    peer_id             TEXT NOT NULL,
    display_name        TEXT NOT NULL DEFAULT '',
    address             TEXT NOT NULL,
    status              TEXT NOT NULL,
    manual              INTEGER NOT NULL DEFAULT 0,
    token_authenticated INTEGER NOT NULL DEFAULT 0,
    last_seen_at        INTEGER NOT NULL DEFAULT 0,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    UNIQUE(peer_id, address)
);
CREATE INDEX IF NOT EXISTS idx_federation_peer_status ON federation_peer(status, peer_id);
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
CREATE TABLE IF NOT EXISTS scrobble_queue (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    service    TEXT NOT NULL,            -- 'listenbrainz' | 'lastfm'
    track_id   INTEGER NOT NULL,
    played_at  INTEGER NOT NULL,         -- unix seconds (the listen timestamp)
    attempts   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_scrobble_service ON scrobble_queue(service, id);
CREATE TABLE IF NOT EXISTS service_auth (
    service     TEXT PRIMARY KEY,   -- 'lastfm'
    session_key TEXT NOT NULL,
    username    TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS station (
    stream_id TEXT PRIMARY KEY REFERENCES stream(id),
    genre     TEXT NOT NULL,
    threshold INTEGER NOT NULL,
    batch     INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS session (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_user ON session(user_id);
CREATE TABLE IF NOT EXISTS invite (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash  TEXT NOT NULL UNIQUE,
    email       TEXT NOT NULL DEFAULT '',
    is_admin    INTEGER NOT NULL DEFAULT 0,
    created_by  INTEGER REFERENCES user(id),
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL,
    accepted_at INTEGER NOT NULL DEFAULT 0
);
