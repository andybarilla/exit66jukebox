import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createStore } from './store.svelte.js';

// createStore reads localStorage for the saved display name; stub it (node env).
beforeEach(() => {
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
  global.fetch = vi.fn();
});
afterEach(() => { vi.restoreAllMocks(); });

describe('auth state', () => {
  it('isAdmin is false when no user is set', () => {
    const s = createStore();
    expect(s.isAdmin).toBe(false);
    expect(s.me).toBe(null);
  });

  it('setMe makes isAdmin reflect the user role', () => {
    const s = createStore();
    s.setMe({ id: 1, email: 'a@b.com', is_admin: true });
    expect(s.isAdmin).toBe(true);
    s.setMe({ id: 2, email: 'b@b.com', is_admin: false });
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

describe('config state', () => {
  it('parses security mode entry-flow config', async () => {
    fetch.mockImplementation((url) => {
      if (url === '/api/config') return Promise.resolve({ ok: true, json: () => Promise.resolve({ security_mode: 'household_profiles', guest_access: false, requires_profile: true, requires_login: false, signup_enabled: false, needs_bootstrap: false, authenticated: false }) });
      if (url === '/api/auth/me') return Promise.resolve({ ok: false, json: () => Promise.resolve({}) });
      return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
    });
    const s = createStore();

    await s.bootstrap();

    expect(s.config.securityMode).toBe('household_profiles');
    expect(s.config.requiresProfile).toBe(true);
    expect(s.config.requiresLogin).toBe(false);
    expect(s.config.guestAccess).toBe(false);
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

describe('personal stream availability (#128)', () => {
  it('assumes there is none until config says otherwise', () => {
    const s = createStore();
    expect(s.hasPersonalStream).toBe(false);
  });

  it('reflects personal_stream from /api/config', async () => {
    const s = createStore();
    global.fetch = vi.fn(async (url) => {
      if (String(url).startsWith('/api/config')) {
        return { ok: true, json: async () => ({ personal_stream: true, security_mode: 'full_login' }) };
      }
      return { ok: true, json: async () => ({}) };
    });
    await s.bootstrap();
    expect(s.hasPersonalStream).toBe(true);
  });

  it('toggleStream does not switch to a personal stream that does not exist', () => {
    const s = createStore();
    const before = s.stream;
    s.toggleStream();
    expect(s.stream).toBe(before);
  });
});
