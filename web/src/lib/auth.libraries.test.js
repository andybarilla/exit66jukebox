import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  addFederationPeer,
  approveFederationPeer,
  getFederationPeers,
  getLibraries,
  listLibraryPaths,
  setLibraries,
} from './auth.js';

afterEach(() => vi.restoreAllMocks());

describe('library admin api', () => {
  test('reads library settings from admin endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ json: async () => ({ local_libraries: [] }) })));
    await expect(getLibraries()).resolves.toEqual({ local_libraries: [] });
    expect(fetch).toHaveBeenCalledWith('/api/admin/libraries');
  });

  test('saves library settings as json', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ ok: true }) })));
    await expect(setLibraries({ save_and_scan: true })).resolves.toEqual({ ok: true });
    expect(fetch).toHaveBeenCalledWith('/api/admin/libraries', expect.objectContaining({ method: 'POST' }));
  });

  test('manages direct federation peers', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ peers: [] }) })));
    await expect(getFederationPeers()).resolves.toEqual({ peers: [] });
    await addFederationPeer({ peer_id: 'peer-a', address: '127.0.0.1:9443' });
    await approveFederationPeer('peer-a');
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/admin/federation/peers');
    expect(fetch).toHaveBeenNthCalledWith(2, '/api/admin/federation/peers', expect.objectContaining({ method: 'POST' }));
    expect(fetch).toHaveBeenNthCalledWith(3, '/api/admin/federation/peers/peer-a/approve', expect.objectContaining({ method: 'POST' }));
  });

  test('lists library paths from the default server start', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ path: '/srv/music', directories: [] }) })));
    await expect(listLibraryPaths()).resolves.toEqual({ path: '/srv/music', directories: [] });
    expect(fetch).toHaveBeenCalledWith('/api/admin/library-paths');
  });

  test('lists library paths with an encoded server path', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ path: '/tmp/Music A', directories: [] }) })));
    await listLibraryPaths('/tmp/Music A');
    expect(fetch).toHaveBeenCalledWith('/api/admin/library-paths?path=%2Ftmp%2FMusic%20A');
  });

  test('throws library path browser errors from json response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, json: async () => ({ error: 'path is not a directory' }) })));
    await expect(listLibraryPaths('/tmp/file')).rejects.toThrow('path is not a directory');
  });
});
