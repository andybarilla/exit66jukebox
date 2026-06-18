import { afterEach, describe, expect, test, vi } from 'vitest';
import { addFederationPeer, approveFederationPeer, getFederationPeers, getLibraries, setLibraries } from './auth.js';

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
});
