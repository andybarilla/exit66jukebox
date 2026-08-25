import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import {
  listTracks, listAlbums, listArtists, discoverRecommended,
  getSonosVolume, setSonosVolume, addManualSonos, nextShared, nextTrack, getConfig, HOUSE,
  removeRequest, setShuffle, castSonos, startStation, stopStation,
} from './api.js';

function mockFetch(items, totalHeader) {
  global.fetch = vi.fn(async (url) => ({
    json: async () => items,
    headers: { get: (k) => (k === 'X-Total-Count' ? totalHeader : null) },
    _url: url,
  }));
}

afterEach(() => { vi.restoreAllMocks(); });

describe('paged list api', () => {
  it('returns items plus the X-Total-Count total and passes search/offset/limit', async () => {
    mockFetch([{ id: 1 }], '42');
    const r = await listTracks('blue', 20, 10);
    expect(r.items).toEqual([{ id: 1 }]);
    expect(r.total).toBe(42);
    const url = global.fetch.mock.calls[0][0];
    expect(url).toContain('/api/tracks');
    expect(url).toContain('search=blue');
    expect(url).toContain('offset=20');
    expect(url).toContain('limit=10');
  });

  it('falls back to items.length when the header is absent', async () => {
    mockFetch([{ id: 1 }, { id: 2 }], null);
    const r = await listAlbums('', 0, 50);
    expect(r.total).toBe(2);
  });

  it('tolerates a non-array body', async () => {
    mockFetch({ error: 'x' }, null);
    const r = await listArtists('', 0, 50);
    expect(r.items).toEqual([]);
    expect(r.total).toBe(0);
  });
});

describe('discoverRecommended', () => {
  it('GETs the recommended endpoint and returns the array body', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => [{ id: 7 }] }));
    const r = await discoverRecommended();
    expect(global.fetch.mock.calls[0][0]).toBe('/api/discover/recommended');
    expect(r).toEqual([{ id: 7 }]);
  });

  it('tolerates a non-array body', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ error: 'x' }) }));
    const r = await discoverRecommended();
    expect(r).toEqual([]);
  });
});

describe('sonos volume + manual ip', () => {
  it('getSonosVolume GETs with the ip query param', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ volume: 33 }) }));
    const r = await getSonosVolume('192.168.1.5');
    expect(global.fetch.mock.calls[0][0]).toBe('/api/sonos/volume?ip=192.168.1.5');
    expect(r.volume).toBe(33);
  });

  it('setSonosVolume POSTs ip + volume as form body', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ ok: true }) }));
    await setSonosVolume('192.168.1.5', 80);
    const [url, opts] = global.fetch.mock.calls[0];
    expect(url).toBe('/api/sonos/volume');
    expect(opts.method).toBe('POST');
    expect(opts.body.get('ip')).toBe('192.168.1.5');
    expect(opts.body.get('volume')).toBe('80');
  });

  it('castSonos POSTs the ip and the stream to cast', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ ok: true }) }));
    await castSonos('192.168.1.5', 'party01');
    const [url, opts] = global.fetch.mock.calls[0];
    expect(url).toBe('/api/sonos/cast');
    expect(opts.body.get('ip')).toBe('192.168.1.5');
    expect(opts.body.get('stream')).toBe('party01');
  });

  it('addManualSonos resolves to {name, ip} on success', async () => {
    global.fetch = vi.fn(async () => ({ ok: true, json: async () => ({ name: 'Kitchen', ip: '192.168.1.7' }) }));
    const r = await addManualSonos('192.168.1.7');
    expect(global.fetch.mock.calls[0][0]).toBe('/api/sonos/manual');
    expect(r).toEqual({ name: 'Kitchen', ip: '192.168.1.7' });
  });

  it('addManualSonos throws when the server rejects the ip', async () => {
    global.fetch = vi.fn(async () => ({ ok: false, json: async () => ({}) }));
    await expect(addManualSonos('8.8.8.8')).rejects.toThrow();
  });

  it('nextShared POSTs to advance the named stream', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ ok: true }) }));
    await nextShared(HOUSE);
    expect(global.fetch.mock.calls[0]).toEqual(['/api/streams/house/next', { method: 'POST' }]);
  });

  it('nextTrack POSTs to advance the private stream', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ ok: true }) }));
    await nextTrack();
    expect(global.fetch.mock.calls[0]).toEqual(['/api/streams/me/next', { method: 'POST' }]);
  });
});

describe('getConfig', () => {
  it('GETs /api/config and returns the parsed body', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ mute_local_on_cast: true }) }));
    const r = await getConfig();
    expect(global.fetch.mock.calls[0][0]).toBe('/api/config');
    expect(r).toEqual({ mute_local_on_cast: true });
  });
});

// #174: the station writes report the outcome rather than handing back a body
// the caller reads as success. The error text is the server's when it sent one
// and a fallback otherwise, which is also what a non-JSON body resolves to.
describe('station writes', () => {
  it('startStation POSTs the genre and reports success', async () => {
    global.fetch = vi.fn(async () => ({ ok: true, json: async () => ({ stream_id: HOUSE, genre: 'Rock' }) }));
    const r = await startStation(HOUSE, 'Rock');
    const [url, opts] = global.fetch.mock.calls[0];
    expect(url).toBe('/api/streams/house/station');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ genre: 'Rock' });
    expect(r).toEqual({ ok: true });
  });

  it('stopStation DELETEs and reports success', async () => {
    global.fetch = vi.fn(async () => ({ ok: true, json: async () => ({ ok: true }) }));
    const r = await stopStation(HOUSE);
    const [url, opts] = global.fetch.mock.calls[0];
    expect(url).toBe('/api/streams/house/station');
    expect(opts.method).toBe('DELETE');
    expect(r).toEqual({ ok: true });
  });

  it('carries the server error message on a refusal', async () => {
    global.fetch = vi.fn(async () => ({ ok: false, status: 403, json: async () => ({ error: 'admin required' }) }));
    expect(await startStation(HOUSE, 'Rock')).toEqual({ ok: false, error: 'admin required' });
    expect(await stopStation(HOUSE)).toEqual({ ok: false, error: 'admin required' });
  });

  it('falls back to its own message when the error body is not JSON', async () => {
    global.fetch = vi.fn(async () => ({ ok: false, status: 404, json: async () => { throw new SyntaxError('Unexpected token <'); } }));
    expect(await startStation(HOUSE, 'Rock')).toEqual({ ok: false, error: 'could not start the station' });
    expect(await stopStation(HOUSE)).toEqual({ ok: false, error: 'could not stop the station' });
  });
});
