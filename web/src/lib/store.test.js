import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createStore } from './store.svelte.js';
import { HOUSE } from './api.js';

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

describe('cast state', () => {
  it('starts with nothing cast and records the streams being cast', () => {
    const s = createStore();
    expect(s.castStreams).toEqual([]);
    s.setCastStreams(['house', 'party01']);
    expect(s.castStreams).toEqual(['house', 'party01']);
    s.setCastStreams([]);
    expect(s.castStreams).toEqual([]);
  });

  // The mute rule is per stream: a cast of some other stream is somebody else's
  // speaker and must leave this listener's local audio alone.
  it('castingStream is true only for the stream being cast', () => {
    const s = createStore();
    expect(s.castingStream(HOUSE)).toBe(false);
    s.setCastStreams(['party01']);
    expect(s.castingStream('party01')).toBe(true);
    expect(s.castingStream(HOUSE)).toBe(false);
  });

  it('setCastStreams ignores a non-array', () => {
    const s = createStore();
    s.setCastStreams(null);
    expect(s.castStreams).toEqual([]);
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

  // The regression this guards: config is fetched at mount, when the visitor is
  // logged out and personal_stream is false. Login runs start(), not
  // bootstrap(), so without a re-read the Personal control stays hidden until
  // the user reloads the page — in full_login, the primary mode.
  it('re-reads config in start(), so logging in reveals the personal stream', async () => {
    const s = createStore();
    let authed = false;
    const headers = { get: () => null };
    // start() ends by opening the SSE subscription; node has no EventSource.
    vi.stubGlobal('EventSource', class { close() {} });
    global.fetch = vi.fn(async (url) => {
      const u = String(url);
      if (u.startsWith('/api/config')) {
        return { ok: true, headers, json: async () => ({ personal_stream: authed, security_mode: 'full_login' }) };
      }
      if (u.startsWith('/api/scan')) return { ok: true, headers, json: async () => ({ running: false }) };
      return { ok: true, headers, json: async () => ([]) };
    });

    await s.bootstrap();                 // mount, logged out
    expect(s.hasPersonalStream).toBe(false);

    authed = true;                       // the user logs in
    await s.start();
    expect(s.hasPersonalStream).toBe(true);
  });

  it('toggleStream does not switch to a personal stream that does not exist', () => {
    const s = createStore();
    const before = s.stream;
    s.toggleStream();
    expect(s.stream).toBe(before);
  });
});

// #142: GET /api/streams/{id} now 404s an id with no row, where it used to
// answer 200 with an empty queue. start() awaits that read inside a
// Promise.all and opens the SSE connection on the line after, so a rejection
// there would cost the session its live updates with nothing shown to the
// user. A proxy answering the 404 with an HTML error page is how the body
// stops being JSON.
describe('a stream read whose body is not JSON (#142)', () => {
  it('still lets start() subscribe to the stream', async () => {
    let opened = 0;
    vi.stubGlobal('EventSource', class {
      constructor() { opened++; }
      close() {}
    });
    global.fetch = vi.fn(async (url) => {
      if (String(url) === '/api/streams/house') {
        return { ok: false, status: 404, json: async () => { throw new SyntaxError('Unexpected token <'); } };
      }
      if (String(url).startsWith('/api/config')) {
        return { ok: true, json: async () => ({ security_mode: 'open', guest_access: true, authenticated: false }) };
      }
      return { ok: true, headers: { get: () => null }, json: async () => [] };
    });
    const store = createStore();

    await store.start();

    expect(opened).toBe(1);
    expect(store.queue).toEqual([]);
    expect(store.toasts).toEqual([]);
    store.teardown();
  });
});

// #160: GET /api/streams/{id}/station now 404s an id with no row, where it used
// to answer 200 {} for any id. loadDiscover awaits that read inside a
// Promise.all and the discover tab calls loadDiscover without a catch, so a
// rejection there would abandon the tab's load with nothing shown to the user
// and leave a stale station on screen. A proxy answering the 404 with an HTML
// error page is how the body stops being JSON.
describe('a station read whose body is not JSON (#160)', () => {
  const stationURL = `/api/streams/${HOUSE}/station`;

  function mockDiscover(stationResponse) {
    global.fetch = vi.fn(async (url) => {
      const u = String(url);
      if (u === stationURL) return stationResponse();
      if (u.startsWith('/api/discover/rediscover')) return { ok: true, json: async () => [{ id: 7, title: 'Rediscovered' }] };
      return { ok: true, json: async () => [] };
    });
  }

  it('clears the station and still loads the tab', async () => {
    // Seed a live station first, so the null below is a value this code put
    // back rather than the state the store starts in.
    mockDiscover(() => ({ ok: true, json: async () => ({ stream_id: HOUSE, genre: 'Rock', threshold: 3, batch: 10 }) }));
    const s = createStore();
    await s.loadDiscover();
    expect(s.discoverStation?.genre).toBe('Rock');

    mockDiscover(() => ({ ok: false, status: 404, json: async () => { throw new SyntaxError('Unexpected token <'); } }));

    await s.loadDiscover();

    expect(s.discoverStation).toBe(null);
    expect(s.discoverRediscover.map((t) => t.title)).toEqual(['Rediscovered']);
    expect(s.toasts).toEqual([]);
  });
});

// #174: POST/DELETE /api/streams/{id}/station both return an error body the
// store used to ignore — it toasted "Station started" on a refused start, and
// cleared the displayed station on a refused stop. streamGate 404s an id with
// no row (a stream deleted since the tab loaded); requireAdmin 403s a non-admin
// on a shared stream. A proxy answering either with an HTML error page is how
// the body stops being JSON.
describe('a refused station write (#174)', () => {
  const stationURL = `/api/streams/${HOUSE}/station`;

  // GET and DELETE share the station URL, so the mock branches on method: the
  // read has to succeed for a seeded station to exist for the stop to leave.
  function mockStation({ get, write }) {
    global.fetch = vi.fn(async (url, init) => {
      const u = String(url);
      const method = init?.method || 'GET';
      if (u === stationURL) return method === 'GET' ? get() : write();
      return { ok: true, json: async () => [] };
    });
  }

  const liveStation = () => ({ ok: true, json: async () => ({ stream_id: HOUSE, genre: 'Rock', threshold: 3, batch: 10 }) });
  const refused = () => ({ ok: false, status: 404, json: async () => ({ error: 'no such stream' }) });
  const refusedNonJSON = () => ({ ok: false, status: 404, json: async () => { throw new SyntaxError('Unexpected token <'); } });

  it('toasts the error instead of "Station started" when the start is refused', async () => {
    mockStation({ get: () => ({ ok: true, json: async () => ({}) }), write: refused });
    const s = createStore();

    await s.startStation('Rock');

    expect(s.toasts.map((t) => t.title)).toEqual(['Not started']);
    expect(s.toasts[0].message).toBe('no such stream');
    expect(s.discoverStation).toBe(null);
  });

  it('leaves the displayed station alone when the stop is refused', async () => {
    // Seed a live station first, so the assertion below is state this code kept
    // rather than the null a fresh store starts in.
    mockStation({ get: liveStation, write: refused });
    const s = createStore();
    await s.loadDiscover();
    expect(s.discoverStation?.genre).toBe('Rock');

    await s.stopStation();

    expect(s.discoverStation?.genre).toBe('Rock');
    expect(s.toasts.map((t) => t.title)).toEqual(['Not stopped']);
    expect(s.toasts[0].message).toBe('no such stream');
  });

  it('does not reject out of either action when the error body is not JSON', async () => {
    mockStation({ get: () => ({ ok: true, json: async () => ({}) }), write: refusedNonJSON });
    const s = createStore();

    await s.startStation('Rock');
    await s.stopStation();

    expect(s.toasts.map((t) => t.title)).toEqual(['Not started', 'Not stopped']);
    expect(s.toasts.map((t) => t.message)).toEqual([
      'could not start the station', 'could not stop the station',
    ]);
  });
});
