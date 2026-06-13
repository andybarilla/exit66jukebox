const SESSION = 'me'; // single private stream id for v1; replaced by real session later

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
  const r = await fetch(`/api/streams/${SESSION}/next`);
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

export async function getQueue(streamId) {
  const r = await fetch(`/api/streams/${streamId}`);
  return r.json(); // { id, queue: [...] }
}

export function houseStreamURL() {
  return `/stream/${HOUSE}.mp3`;
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

export async function removeRequest(streamId, trackId) {
  const r = await fetch(`/api/streams/${streamId}/requests/${trackId}`, { method: 'DELETE' });
  return r.json();
}

export async function setShuffle(streamId, on) {
  const body = new URLSearchParams({ value: on ? 'true' : 'false' });
  const r = await fetch(`/api/streams/${streamId}/shuffle`, { method: 'POST', body });
  return r.json();
}

export function albumCoverURL(albumId) { return `/api/albums/${albumId}/cover`; }
