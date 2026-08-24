// PERSONAL is an alias, not a stream id: the server resolves it to the caller's
// own personal stream and provisions one on first use (#128). The client never
// learns or chooses a personal stream id, and supplying one directly is refused.
// Whether a personal stream exists at all is reported by /api/config as
// personal_stream — it does not in the two open security modes.
const SESSION = 'me';

// listPage fetches one page of a browse list, returning the rows plus the
// unpaged total from X-Total-Count (falling back to the page length when the
// header is absent so the caller's paging still terminates).
async function listPage(path, search, offset, limit) {
  const r = await fetch(
    `${path}?search=${encodeURIComponent(search)}&offset=${offset}&limit=${limit}`);
  const body = await r.json();
  const items = Array.isArray(body) ? body : [];
  const raw = r.headers.get('X-Total-Count');
  const total = raw == null || raw === '' ? NaN : Number(raw);
  return { items, total: Number.isFinite(total) ? total : items.length };
}

export const listTracks = (search = '', offset = 0, limit = 100) =>
  listPage('/api/tracks', search, offset, limit);
export const listAlbums = (search = '', offset = 0, limit = 100) =>
  listPage('/api/albums', search, offset, limit);
export const listArtists = (search = '', offset = 0, limit = 100) =>
  listPage('/api/artists', search, offset, limit);

// albumTracks returns one album's enriched tracks for the album dialog.
export async function albumTracks(albumId) {
  const r = await fetch(`/api/albums/${albumId}/tracks`);
  const body = await r.json();
  return Array.isArray(body) ? body : [];
}
export async function requestTrack(trackId) {
  const body = new URLSearchParams({ kind: 'track', id: String(trackId) });
  const r = await fetch(`/api/streams/${SESSION}/requests`, { method: 'POST', body });
  return r.json();
}
export async function nextTrack() {
  const r = await fetch(`/api/streams/${SESSION}/next`, { method: 'POST' });
  return r.json();
}
export function audioURL(trackId) {
  return `/api/tracks/${trackId}/audio`;
}

// scanStatus reports library-scan progress, or null when scanning isn't
// available (no library configured → 503).
export async function scanStatus() {
  const r = await fetch('/api/scan');
  if (!r.ok) return null;
  return r.json(); // {running, added, updated, skipped, failed}
}

export const HOUSE = 'house';
export const PERSONAL = SESSION;

// listStreams returns the shared streams (house included). The personal stream
// is never in this list; the client pins it separately.
export async function listStreams() {
  const r = await fetch('/api/streams');
  const body = await r.json();
  return Array.isArray(body) ? body : [];
}

// createStream makes a named shared stream. Resolves to {ok, stream, error}
// so the caller can surface the cap 409 by its message rather than guessing.
export async function createStream(name) {
  const r = await fetch('/api/streams', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  const body = await r.json().catch(() => ({}));
  return r.ok ? { ok: true, stream: body } : { ok: false, error: body.error || 'could not create the stream' };
}

export async function renameStream(streamId, name) {
  const r = await fetch(`/api/streams/${streamId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  const body = await r.json().catch(() => ({}));
  return r.ok ? { ok: true } : { ok: false, error: body.error || 'could not rename the stream' };
}

export async function deleteStream(streamId) {
  const r = await fetch(`/api/streams/${streamId}`, { method: 'DELETE' });
  const body = await r.json().catch(() => ({}));
  return r.ok ? { ok: true } : { ok: false, error: body.error || 'could not delete the stream' };
}

export async function getQueue(streamId) {
  const r = await fetch(`/api/streams/${streamId}`);
  return r.json(); // { id, queue: [...] }
}

// streamAudioURL is the continuous MP3 feed for a shared stream.
export function streamAudioURL(streamId) {
  return `/stream/${streamId}.mp3`;
}

export function coverURL(trackId) {
  return `/api/tracks/${trackId}/cover`;
}

export async function listSonos() {
  const r = await fetch('/api/sonos/devices');
  return r.json(); // [{name, ip}]
}
export async function castSonos(ip) {
  const r = await fetch('/api/sonos/cast', { method: 'POST', body: new URLSearchParams({ ip }) });
  return r.json();
}
export async function stopSonos(ip) {
  const r = await fetch('/api/sonos/stop', { method: 'POST', body: new URLSearchParams({ ip }) });
  return r.json();
}
export async function getSonosVolume(ip) {
  const r = await fetch(`/api/sonos/volume?ip=${encodeURIComponent(ip)}`);
  return r.json(); // { volume }
}
export async function setSonosVolume(ip, volume) {
  const body = new URLSearchParams({ ip, volume: String(volume) });
  const r = await fetch('/api/sonos/volume', { method: 'POST', body });
  return r.json();
}
// addManualSonos registers a hand-entered Sonos IP for networks where SSDP
// multicast is blocked; resolves to {name, ip} on success and throws otherwise.
export async function addManualSonos(ip) {
  const r = await fetch('/api/sonos/manual', { method: 'POST', body: new URLSearchParams({ ip }) });
  if (!r.ok) throw new Error('not a Sonos device');
  return r.json(); // { name, ip }
}
// nextShared advances a shared stream's queue (the transport's Next button).
export async function nextShared(streamId) {
  const r = await fetch(`/api/streams/${streamId}/next`, { method: 'POST' });
  return r.json();
}


export async function discoverRediscover(genre = '') {
  const r = await fetch(`/api/discover/rediscover?genre=${encodeURIComponent(genre)}`);
  return r.json();
}
export async function discoverRecent(genre = '') {
  const r = await fetch(`/api/discover/recent?genre=${encodeURIComponent(genre)}`);
  return r.json();
}
export async function discoverGenres() {
  const r = await fetch('/api/discover/genres');
  return r.json(); // [{genre, count}]
}
// discoverRecommended returns externally-sourced recommendations mapped to local
// tracks (enriched). Empty when no service is configured or none mapped yet.
export async function discoverRecommended() {
  const r = await fetch('/api/discover/recommended');
  const body = await r.json();
  return Array.isArray(body) ? body : [];
}
export async function getStation(streamId) {
  const r = await fetch(`/api/streams/${streamId}/station`);
  return r.json(); // {stream_id, genre, threshold, batch} or {}
}
export async function startStation(streamId, genre) {
  const r = await fetch(`/api/streams/${streamId}/station`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ genre }),
  });
  return r.json();
}
export async function stopStation(streamId) {
  const r = await fetch(`/api/streams/${streamId}/station`, { method: 'DELETE' });
  return r.json();
}

// subscribeEvents opens an SSE connection; onEvent gets parsed {type,data}.
// Returns a close function.
export function subscribeEvents(streamId, onEvent) {
  const es = new EventSource(`/api/streams/${streamId}/events`);
  es.onmessage = (m) => {
    try { onEvent(JSON.parse(m.data)); } catch (_) {}
  };
  return () => es.close();
}

// requestTo sends the requester name and a kind (track|album|artist).
export async function requestTo(streamId, id, { kind = 'track', by = 'You' } = {}) {
  const body = new URLSearchParams({ kind, id: String(id), by });
  const r = await fetch(`/api/streams/${streamId}/requests`, { method: 'POST', body });
  return r.json();
}

// removeRequest/clearQueue/setShuffle are admin-gated on every shared stream
// and open on a guest's own stream; the session cookie authenticates server-side.
export async function removeRequest(streamId, trackId) {
  const r = await fetch(`/api/streams/${streamId}/requests/${trackId}`, { method: 'DELETE' });
  return r.json();
}

export async function clearQueue(streamId) {
  const r = await fetch(`/api/streams/${streamId}/requests`, { method: 'DELETE' });
  return r.json();
}

export async function setShuffle(streamId, on) {
  const body = new URLSearchParams({ value: on ? 'true' : 'false' });
  const r = await fetch(`/api/streams/${streamId}/shuffle`, { method: 'POST', body });
  return r.json();
}

export function albumCoverURL(albumId) { return `/api/albums/${albumId}/cover`; }

// getConfig returns runtime settings.
export async function getConfig() {
  const r = await fetch('/api/config');
  // return shape includes { mute_local_on_cast, fed_peers, authenticated, is_admin,
  // security_mode, guest_access, requires_profile, requires_login, signup_enabled,
  // needs_bootstrap }
  return r.json();
}
