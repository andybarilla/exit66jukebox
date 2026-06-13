import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  listTracks, listAlbums, listArtists, discoverRecommended,
  getSonosVolume, setSonosVolume, addManualSonos, nextHouse,
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

  it('nextHouse advances the house stream', async () => {
    global.fetch = vi.fn(async () => ({ json: async () => ({ ok: true }) }));
    await nextHouse();
    expect(global.fetch.mock.calls[0][0]).toBe('/api/streams/house/next');
  });
});
