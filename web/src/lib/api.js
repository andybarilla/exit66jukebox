const SESSION = 'me'; // single private stream id for v1; replaced by real session later

// The library is loaded once and filtered client-side, so request the whole
// collection. The cap is high enough to cover a large home library; tracks,
// albums and artists must share the same ceiling or a track whose album fell
// outside the cap would be dropped from the grouped view.
const LIBRARY_LIMIT = 100000;

export async function listTracks(search = '') {
  const r = await fetch(`/api/tracks?search=${encodeURIComponent(search)}&limit=${LIBRARY_LIMIT}`);
  return r.json();
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

export async function listArtists(search = '') {
  const r = await fetch(`/api/artists?search=${encodeURIComponent(search)}&limit=${LIBRARY_LIMIT}`);
  return r.json();
}
export async function listAlbums(search = '') {
  const r = await fetch(`/api/albums?search=${encodeURIComponent(search)}&limit=${LIBRARY_LIMIT}`);
  return r.json();
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
