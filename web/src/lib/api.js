const SESSION = 'me'; // single private stream id for v1; replaced by real session later

export async function listTracks(search = '') {
  const r = await fetch(`/api/tracks?search=${encodeURIComponent(search)}`);
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
  const r = await fetch(`/api/artists?search=${encodeURIComponent(search)}&limit=500`);
  return r.json();
}
export async function listAlbums(search = '') {
  const r = await fetch(`/api/albums?search=${encodeURIComponent(search)}&limit=500`);
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
