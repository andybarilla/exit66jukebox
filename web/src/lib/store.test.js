import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createStore } from './store.svelte.js';

// createStore reads localStorage for the saved display name; stub it (node env).
beforeEach(() => {
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
});
afterEach(() => { vi.restoreAllMocks(); });

describe('admin state', () => {
  it('isAdmin is true by default (gate open until config says otherwise)', () => {
    const s = createStore();
    expect(s.isAdmin).toBe(true);
    expect(s.adminRequired).toBe(false);
  });

  it('becomes locked when config reports admin_required with no valid token', () => {
    const s = createStore();
    s.applyAdminConfig({ admin_required: true, is_admin: false });
    expect(s.adminRequired).toBe(true);
    expect(s.isAdmin).toBe(false);
  });

  it('is unlocked when a valid token makes config report is_admin', () => {
    vi.stubGlobal('localStorage', { getItem: () => 'tok-abc', setItem: () => {}, removeItem: () => {} });
    const s = createStore();
    s.applyAdminConfig({ admin_required: true, is_admin: true });
    expect(s.adminRequired).toBe(true);
    expect(s.isAdmin).toBe(true);
  });

  it('drops a stale token when config says the gate is on but is_admin is false', () => {
    const removed = [];
    vi.stubGlobal('localStorage', {
      getItem: () => 'stale', setItem: () => {}, removeItem: (k) => removed.push(k),
    });
    const s = createStore();
    s.applyAdminConfig({ admin_required: true, is_admin: false });
    expect(removed).toContain('e66.admin');
    expect(s.isAdmin).toBe(false);
  });
});

describe('cast-active state', () => {
  it('defaults to false and is toggled by setCastActive', () => {
    const s = createStore();
    expect(s.castActive).toBe(false);
    s.setCastActive(true);
    expect(s.castActive).toBe(true);
    s.setCastActive(false);
    expect(s.castActive).toBe(false);
  });

  it('exposes muteLocalOnCast, defaulting true before config loads', () => {
    const s = createStore();
    expect(s.muteLocalOnCast).toBe(true);
  });
});

// openAlbum maps the album's enriched track rows for the dialog. The backend
// serves the track position as `track_no`; the dialog renders it, so mapTrack
// must carry it through as `trackNo` (issue #68).
describe('openAlbum track mapping', () => {
  it('carries track_no through as trackNo for the album dialog', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => [
        { id: 1, title: 'Come Together', track_no: 1, duration: 200 },
        { id: 2, title: 'Something', track_no: 2, duration: 180 },
      ],
    }));
    const store = createStore();
    await store.openAlbum({ id: 10, name: 'Abbey Road', artistName: 'The Beatles' });
    expect(store.detailAlbum.tracks.map((t) => t.trackNo)).toEqual([1, 2]);
  });

  it('defaults a missing/zero track_no to 0 so the row degrades gracefully', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => [{ id: 3, title: 'Untitled', duration: 0 }],
    }));
    const store = createStore();
    await store.openAlbum({ id: 11, name: 'X', artistName: 'Y' });
    expect(store.detailAlbum.tracks[0].trackNo).toBe(0);
  });

  // The backend serves comment-derived URLs as a `links` array; the dialog
  // renders them, so mapTrack must carry them through, defaulting to [] (#46).
  it('carries links through for the album dialog, defaulting to []', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => [
        { id: 1, title: 'One', track_no: 1, links: ['https://a.bandcamp.com/x'] },
        { id: 2, title: 'Two', track_no: 2 },
      ],
    }));
    const store = createStore();
    await store.openAlbum({ id: 12, name: 'X', artistName: 'Y' });
    expect(store.detailAlbum.tracks.map((t) => t.links)).toEqual([
      ['https://a.bandcamp.com/x'],
      [],
    ]);
  });
});

// Now-playing and queue payloads are enriched tracks that carry album_id; the
// store surfaces it as albumId so the bar/Lineup/queue covers can openAlbum (#71).
describe('album linkage from now-playing and queue', () => {
  it('surfaces album_id as albumId on queue items and now-playing', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => ({
        id: 'house',
        now_playing: { track: { id: 1, title: 'NP', album_id: 100, album_name: 'NP Album' }, offset_seconds: 0 },
        queue: [{ track: { id: 2, title: 'Q', album_id: 200, album_name: 'Q Album' } }],
      }),
    }));
    const store = createStore();
    await store.refreshQueue('house');
    expect(store.nowPlaying.albumId).toBe(100);
    expect(store.queue[0].albumId).toBe(200);
  });

  it('leaves albumId undefined when the payload lacks album_id', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => ({
        id: 'house',
        now_playing: { track: { id: 1, title: 'NP' }, offset_seconds: 0 },
        queue: [{ track: { id: 2, title: 'Q' } }],
      }),
    }));
    const store = createStore();
    await store.refreshQueue('house');
    expect(store.nowPlaying.albumId).toBeUndefined();
    expect(store.queue[0].albumId).toBeUndefined();
  });
});
