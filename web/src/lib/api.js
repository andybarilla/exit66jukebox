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
